# CI Benchmark Integration — as-built

How the fusion loop timing budget is enforced in CI, verified against the live
`spaxel-build` WorkflowTemplate on iad-ci (2026-09-25, after the template-side
parse guards landed — see "Template-side guards" below). This replaces an
earlier draft that said "add this step" in one section and "already wired" in
another; both described the same gate, neither described it accurately.

The gate lives in **jedarden/declarative-config**
(`k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`, ArgoCD app
`argo-workflows-ns-iad-ci`) — outside this repo, never mutated with kubectl.
This repo owns the benchmark and a contract test that keeps the gate honest;
the two sides are reconciled here.

## What is enforced

**File:** `mothership/internal/localizer/fusion/timing_budget_test.go`
**Benchmark:** `BenchmarkFusionLoop` — the full pipeline (phase sanitization →
feature extraction → Fresnel accumulation → peak extraction → UKF update)
over a fixed 600-iteration window (60 s at 10 Hz, 4 nodes, 2 walkers).

| Bound | Value | Enforced by |
|---|---|---|
| Production target | median < 15 ms | `TestTimingBudgetProduction` (go-test leg) |
| CI threshold | median < 30 ms | `TestTimingBudgetProduction` **and** the timing-benchmark gate |
| Hard limit | P99 < 40 ms | both, same split |

The thresholds are asserted twice, in two different CI legs: as a regular test
(`go test ./...`) and as the benchmark gate. Both must agree; if they drift,
the contract test's doc tripwire or a review catches it.

## How CI runs it

The `timing-benchmark` step runs in the **same step group as `go-test`** — in
parallel, not after it (sequential step groups; a red leg starves everything
after it). The step's script:

```bash
# bash -c with `set -euo pipefail`: a go-test failure fails the node instead
# of vanishing behind tee's exit status
cd repo/mothership
go test -bench=BenchmarkFusionLoop -benchtime=10s -count=1 \
  ./internal/localizer/fusion/ 2>&1 | tee /tmp/bench.txt

# Invariant guards BEFORE any threshold comparison: exactly one summary
# line per token, each parsing to a bare positive decimal
median_lines=$(grep -c "Median:" /tmp/bench.txt || true)
[ "$median_lines" -eq 1 ] || { echo "FAIL: expected exactly one 'Median:' line, found '${median_lines}'"; exit 1; }
p99_lines=$(grep -c "P99:" /tmp/bench.txt || true)
[ "$p99_lines" -eq 1 ] || { echo "FAIL: expected exactly one 'P99:' line, found '${p99_lines}'"; exit 1; }

median_ms=$(grep "Median:" /tmp/bench.txt | sed 's/.*Median: \([0-9.]*\)ms.*/\1/')
p99_ms=$(grep "P99:" /tmp/bench.txt | sed 's/.*P99: \([0-9.]*\)ms.*/\1/')

printf '%s' "$median_ms" | grep -Eq '^[0-9]+([.][0-9]+)?$' \
  || { echo "FAIL: parsed Median value '${median_ms}' is not a bare positive decimal"; exit 1; }
printf '%s' "$p99_ms" | grep -Eq '^[0-9]+([.][0-9]+)?$' \
  || { echo "FAIL: parsed P99 value '${p99_ms}' is not a bare positive decimal"; exit 1; }

ci_threshold=30   # ms
hard_limit=40     # ms

# awk, not bc: bc is absent from the image, and a missing comparison tool
# would re-open the silent pass this guard exists to close
median_over=$(awk -v m="$median_ms" -v t="$ci_threshold" 'BEGIN { print (m > t) ? 1 : 0 }')
[ "$median_over" = "1" ] && { echo "FAIL: Median ${median_ms}ms exceeds CI threshold ${ci_threshold}ms"; exit 1; }
p99_over=$(awk -v p="$p99_ms" -v t="$hard_limit" 'BEGIN { print (p > t) ? 1 : 0 }')
[ "$p99_over" = "1" ] && { echo "FAIL: P99 ${p99_ms}ms exceeds hard limit ${hard_limit}ms"; exit 1; }

echo "PASS: Timing constraints satisfied (Median: ${median_ms}ms, P99: ${p99_ms}ms)"
```

Step properties: `retryStrategy` OnError limit 1, `activeDeadlineSeconds: 900`.
No `-run='^$'`: the invocation also runs `TestTimingBudgetProduction` and the
contract test (see below) — ordinary tests always run under `-bench=`.

## The output contract

The parse expects **exactly one** `Median: <n>ms` and one `P99: <n>ms` line in
the whole output, each with a bare decimal before `ms`. Two in-file properties
hold that contract, and both are regression-checked by
`mothership/internal/localizer/fusion/ci_gate_contract_test.go`:

1. **`BenchmarkFusionLoop` returns early on rounds after the first**
   (`if b.N > 1 { return }`). The window is fixed, so the framework's
   `-benchtime` estimate can never converge; before the guard,
   `-benchtime=10s` spun ~30 rounds (up to the b.N 1e9 cap), each printing its
   own summary pair. The sed then returned multi-line values, `bc` errored,
   `(( ))` evaluated false inside `if` — and the gate **passed
   unconditionally**, no matter the timing. This was the gate's real
   failure mode: not red, silent.
2. **`TestTimingBudgetProduction`'s log dump avoids the gate tokens**
   (lowercase `median=` / `p99=`). A failing test's logs print even without
   `-v`, and this test runs in the gate's output — a load-induced failure
   would otherwise inject a second `Median:`/`P99:` pair and disarm the
   threshold check.

The `--- BENCH:` block with those lines prints with or without `-v`.

## The regression check

`ci_gate_contract_test.go` (same package) fails the **go-test leg** when the
benchmark gate is absent or ineffective:

- `TestCIGateOutputContract` re-runs the gate's exact invocation and applies
  the gate's exact grep/sed shape in Go, asserting: `BenchmarkFusionLoop`
  ran, and its output yields exactly one well-formed positive
  `Median`/`P99` pair. It tolerates a timing-threshold failure (a real gate
  signal on a loaded host) but not a missing benchmark or a malformed parse.
  A sentinel env var stops it recursing inside its own subprocess (the
  gate's invocation runs ordinary tests, so it runs this test too).
- `TestBenchmarkGuideDocumentsGate` fails if this doc stops naming
  `BenchmarkFusionLoop` / `timing-benchmark` — the tripwire against the guide
  drifting away from the gate again.

Both skip in `-short` mode. Neither asserts timing values — the budget itself
is asserted by the benchmark and `TestTimingBudgetProduction`; a wall-clock
assertion in `go test ./...` would just be a third host-load detector.

## Template-side guards (declarative-config)

The historical template-side holes are **closed** as of 2026-09-25:
declarative-config commits `5d7987b9` (guards) + `3a81f924` (mechanism-note
correction), bead `spaxel-d8268220`, live on iad-ci via ArgoCD sync the same
day and re-verified against the live template with read-only kubectl:

- **pipefail:** the step now runs `bash -c` with `set -euo pipefail`. A
  go-test failure (build error, benchmark crash) fails the timing node
  instead of vanishing behind `tee`'s exit status. Bash is required because
  POSIX sh (dash on the Debian golang image) has no `pipefail`.
- **exactly-one-pair invariant:** the step counts `Median:`/`P99:` lines and
  exits 1 with a diagnostic naming the count unless each count is exactly
  one.
- **numeric parse guard:** each parsed value must match
  `^[0-9]+([.][0-9]+)?$` or the node fails before any threshold comparison.
  Every violation prints a diagnostic naming the broken invariant — a silent
  PASS on empty or garbled output is no longer reachable.
- **the comparison itself:** awk, not bc (bc is absent from the image; a
  missing comparison tool would re-open the silent pass). Also not `(( ))`:
  the old `if (( ... | bc -l ))` was bash-only arithmetic that POSIX sh
  parses as *nested subshells* — comparison output discarded, `if` keyed on
  the pipeline's exit status (bc absent → 127 → false → skip) — so the
  threshold check never compared either way. That mechanism is recorded in
  the template comment and declarative-config `3a81f924`.

The thresholds (median < 30, P99 < 40), `retryStrategy` OnError limit 1, and
`activeDeadlineSeconds: 900` are unchanged. This repo's contract test
(`TestCIGateOutputContract`) mirrors the invocation and parse shape and stays
the in-repo tripwire: the gate can now fail loudly on its own, and the
contract test keeps this repo's half of the output contract gate-compatible.

## Running locally

```bash
cd mothership

# The CI-shaped run (one 600-iteration window ≈ 2 s; later framework rounds
# are instant no-ops, so -benchtime barely affects wall time)
go test -bench=BenchmarkFusionLoop -benchtime=10s -count=1 ./internal/localizer/fusion/

# The same thresholds as a regular test
go test ./internal/localizer/fusion/

# Everything except the timing-sensitive parts
go test -short ./internal/localizer/fusion/
```

Known local quirk: on a loaded host, `TestTimingBudgetProduction`'s P99 leg
can fail (observed 44–60 ms) while the median stays healthy — that is host
load, not a regression; the median bound and the benchmark gate are the
stable signals. See the project runbook note on the two load-sensitive perf
tests; do not tune thresholds for it.

## Acceptance criteria

The gate does its job when:

- ✅ The benchmark produces exactly one `Median:`/`P99:` pair per CI run
  (checked continuously by `TestCIGateOutputContract` in the go-test leg)
- ✅ Median > 30 ms or P99 > 40 ms fails the CI run (benchmark gate *and*
  `TestTimingBudgetProduction`, in separate legs)
- ✅ A missing, renamed, crashing, or output-malformed benchmark fails the
  go-test leg (contract test) **and** the timing-benchmark node itself
  (pipefail + parse guards) instead of passing silently; the node's PASS/FAIL
  line always carries the actual parsed values

## Performance baselines

i5-13500, 2026-09-24, under normal desktop/CI load:
median **2.0–2.4 ms**, P99 **7.4–8.0 ms** — comfortably under the 15 ms
production target, with ~4× median headroom to the 30 ms CI threshold.

## Changing any of this

- Benchmark, thresholds, output format: this repo — keep
  `ci_gate_contract_test.go` and this doc in sync (the tripwire enforces the
  doc's half).
- The step itself, its thresholds, or its parse: `jedarden/declarative-config`
  → commit → push → ArgoCD sync. Never kubectl. After changing it, re-verify
  this doc against the live template
  (`kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate
  spaxel-build -n argo-workflows -o json`) and update the "verified" date at
  the top.
