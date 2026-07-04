# CI Benchmark Integration Guide

This document describes how to integrate the fusion loop timing benchmark with Argo Workflows CI.

## Overview

The timing benchmark enforces the fusion loop timing budget as a CI quality gate (per plan §Quality Gates / Definition of Done, item 9).

**File:** `internal/localizer/fusion/timing_budget_test.go`

**Benchmark:** `BenchmarkFusionLoop`

**What it tests:** The full fusion pipeline:
1. Phase sanitization → Feature extraction → Fresnel accumulation → Peak extraction → UKF update
2. Against synthetic CSI data from spaxel-sim output (4 nodes, 2 walkers)

**Timing constraints:**
- Median fusion iteration < 15 ms (production target)
- Median fusion iteration < 30 ms (CI threshold - 2x allowance for slower hardware)
- P99 < 40 ms (hard limit)

## Running Locally

```bash
# Run the benchmark (60 seconds)
go test -bench=BenchmarkFusionLoop -benchtime=60s -count=1 ./internal/localizer/fusion/

# Run the regular test (includes timing assertions)
go test -v ./internal/localizer/fusion/
```

## Argo Workflow Integration

Add this step to the spaxel-build Argo WorkflowTemplate after the `go test ./...` step:

```yaml
- name: run-timing-benchmark
  template: spaxel-build
  arguments:
    parameters:
      - name: command
        value: |
          go test -bench=BenchmarkFusionLoop -benchtime=60s -count=1 \
            ./internal/localizer/fusion/ 2>&1 | tee /tmp/bench.txt

          # Parse and check thresholds
          median_ms=$(grep "Median:" /tmp/bench.txt | sed 's/.*Median: \([0-9.]*\)ms.*/\1/')
          p99_ms=$(grep "P99:" /tmp/bench.txt | sed 's/.*P99: \([0-9.]*\)ms.*/\1/')

          ci_threshold=30  # CI threshold in ms
          hard_limit=40    # Hard limit in ms

          if (( $(echo "$median_ms > $ci_threshold" | bc -l) )); then
            echo "FAIL: Median ${median_ms}ms exceeds CI threshold ${ci_threshold}ms"
            exit 1
          fi

          if (( $(echo "$p99_ms > $hard_limit" | bc -l) )); then
            echo "FAIL: P99 ${p99_ms}ms exceeds hard limit ${hard_limit}ms"
            exit 1
          fi

          echo "PASS: Timing constraints satisfied (Median: ${median_ms}ms, P99: ${p99_ms}ms)"
```

## CI Execution

GitHub Actions are disabled across all repos — all CI runs on Argo Workflows (iad-ci). The
timing benchmark is already wired into the `spaxel-build` Argo WorkflowTemplate, which runs
`go test -bench=BenchmarkFusionLoop` as a step (alongside `go test ./...`). There is no
separate GitHub Actions path; the Argo template above is the canonical integration and the
only place this gate runs.

## Acceptance Criteria

The CI gate passes when:
- ✅ Benchmark runs successfully for 600 iterations (60 seconds at 10 Hz)
- ✅ Median fusion iteration < 30 ms on CI runner (2x allowance)
- ✅ P99 fusion iteration < 40 ms (hard limit)

## Performance Baselines

Typical results on reference hardware (13th Gen Intel i5-13500):
- Median: ~3ms (well under 15ms production target)
- P99: ~4ms (well under 40ms hard limit)

These results provide significant headroom for slower CI runners while maintaining the production target.
