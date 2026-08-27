#!/bin/bash
# Run acceptance tests with diagnostic instrumentation to pinpoint hangs

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd")"

cd "$REPO_ROOT"

echo "==================================="
echo "Spaxel Acceptance Test Diagnostics"
echo "==================================="
echo ""
echo "This script runs acceptance tests with enhanced diagnostic logging:"
echo "  - Goroutine dumps every 30 seconds"
echo "  - Phase tracking with timeout markers"
echo "  - IO operation logging"
echo "  - Memory statistics"
echo ""
echo "Diagnostics output will be written to temp files (listed at test end)"
echo ""

# Check if integration test mode is enabled
if [ "$SPAXEL_INTEGRATION_TEST" != "1" ] && [ "$ACCEPTANCE_TEST" != "1" ]; then
    echo "Note: Enabling ACCEPTANCE_TEST=1 for this run"
    export ACCEPTANCE_TEST=1
fi

# Parse which test to run
TEST_NAME="${1:-TestIO1_FreshInstallFirstBoot}"
TEST_TIMEOUT="${2:-5m}"

echo "Running test: $TEST_NAME"
echo "Timeout: $TEST_TIMEOUT"
echo ""

# Run the test with verbose output and timeout
echo "Starting test..."
echo ""

# Set a hard timeout to prevent infinite hangs
timeout "$TEST_TIMEOUT" go test -v -run "^$TEST_NAME$" ./test/acceptance/... 2>&1 | tee /tmp/spaxel-test-output.log || TEST_EXIT_CODE=$?

echo ""
echo "==================================="
echo "Test completed"
echo "==================================="

if [ "${TEST_EXIT_CODE:-0}" -eq 124 ]; then
    echo "❌ TEST TIMED OUT after $TEST_TIMEOUT"
    echo ""
    echo "This indicates the test is hanging. Check the diagnostic output for:"
    echo "  1. Which phase was active when the timeout occurred"
    echo "  2. Goroutine dumps showing what was blocking"
    echo "  3. IO operations that never completed"
    echo ""
    echo "Full test output saved to: /tmp/spaxel-test-output.log"
    echo ""
    echo "Look for diagnostic temp files with pattern: /tmp/spaxel-test-diagnostics-*.txt"
    echo "These contain detailed goroutine stack traces."
    exit 1
elif [ "${TEST_EXIT_CODE:-0}" -ne 0 ]; then
    echo "❌ TEST FAILED with exit code: ${TEST_EXIT_CODE:-0}"
    echo ""
    echo "Full test output saved to: /tmp/spaxel-test-output.log"
    echo ""
    echo "Check for diagnostic files: /tmp/spaxel-test-diagnostics-*.txt"
    exit 1
else
    echo "✅ TEST PASSED"
    echo ""
    echo "Test output saved to: /tmp/spaxel-test-output.log"
    echo "Check diagnostic files for detailed execution trace:"
    find /tmp -name "spaxel-test-diagnostics-*.txt" -mtime -1 2>/dev/null || true
    exit 0
fi
