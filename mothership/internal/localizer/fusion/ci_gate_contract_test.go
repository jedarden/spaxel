package fusion

// Contract tests for the spaxel-build "timing-benchmark" CI gate (iad-ci).
//
// The gate is a shell step in an Argo WorkflowTemplate that lives in
// jedarden/declarative-config — outside this repository — so this repo
// cannot test the template itself. What it CAN test is this repo's half of
// the contract the gate depends on:
//
//  1. The gate is PRESENT: BenchmarkFusionLoop exists, builds, and actually
//     runs (a deleted/renamed benchmark makes the gate check nothing).
//  2. The gate is EFFECTIVE: the benchmark output yields exactly one
//     "Median" and one "P99" summary line, and the gate's grep+sed
//     extraction ("grep Median: | sed 's/.*Median: \([0-9.]*\)ms.*/\1/'")
//     produces a single non-empty, numeric value from them.
//
// Both halves were broken once already: the benchmark ignored b.N, so the
// testing framework re-ran the fixed 600-iteration window until its
// benchtime estimate hit the b.N cap (~30 rounds at -benchtime=10s), each
// round printing its own summary line. The gate's sed then returned
// multi-line values that bc rejected inside an arithmetic if — evaluating
// false, so the gate passed unconditionally no matter the timing. The b.N>1
// guard in BenchmarkFusionLoop and the de-collided log tokens in
// TestTimingBudgetProduction are the fix; these tests are the tripwire.
//
// Known residual holes on the template side (declarative-config, documented
// in docs/ci-benchmark-integration.md, not fixable from this repo): the step
// pipes `go test | tee` without pipefail (swallowing the benchmark's own
// failure exit code) and does not guard against an empty parse when the
// benchmark fails to build. Until those are fixed there, the in-repo
// protection is this file: a broken or absent benchmark fails the go-test
// step, which turns the workflow red even though the (currently
// toothless-on-error) timing step runs in parallel.
//
// Timing VALUES are deliberately not asserted here — the benchmark asserts
// its own thresholds, and a wall-clock assertion in `go test ./...` would be
// another host-load detector like TestTimingBudgetProduction's P99 leg.
//
// Re-entrancy: `go test -bench=...` runs ordinary tests too, so the gate's
// invocation executes this test — which spawns the very same invocation.
// The sentinel env var marks child processes so the recursion stops at
// depth one.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// ciGateContractChildEnv marks a nested `go test` process so this test does
// not recurse inside it (the gate's invocation runs ordinary tests too).
const ciGateContractChildEnv = "SPAXEL_CI_GATE_CONTRACT_CHILD"

// TestCIGateOutputContract runs the exact invocation the spaxel-build
// timing-benchmark step runs and applies the exact parse the step applies,
// then asserts the parse is well-defined (single line, non-empty, numeric).
func TestCIGateOutputContract(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping CI gate contract test in short mode")
	}
	if os.Getenv(ciGateContractChildEnv) != "" {
		t.Skip("nested gate invocation — parent run already checks the contract")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go binary not on PATH: %v", err)
	}

	// Same command line as the live template's timing-benchmark step.
	cmd := exec.Command(goBin, "test",
		"-bench=BenchmarkFusionLoop", "-benchtime=10s", "-count=1",
		"./internal/localizer/fusion/")
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), ciGateContractChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	output := string(out)
	// The benchmark (or TestTimingBudgetProduction, which this invocation
	// also runs) may legitimately fail its own timing assertions on a loaded
	// host — that is a real signal for the gate, not a contract failure.
	// Only a build/setup failure leaves no benchmark result line behind.
	if !strings.Contains(output, "BenchmarkFusionLoop") {
		t.Fatalf("benchmark did not run (gate absent). go test exit: %v\nOutput:\n%s", err, output)
	}

	medianMS := parseGateValue(t, output, "Median")
	p99MS := parseGateValue(t, output, "P99")
	t.Logf("gate parse result: median=%sms p99=%sms (thresholds: median < %v, p99 < %v)",
		medianMS, p99MS, ciThreshold, hardLimit)
}

// parseGateValue mirrors the gate's `grep <token>: | sed 's/.*<token>: \([0-9.]*\)ms.*/\1/'`
// extraction and asserts it yields exactly one well-formed value. A count
// other than one, an empty capture, or a non-numeric capture are all the
// "gate ineffective" shapes: each makes the step's bc comparison a silent
// no-op instead of a threshold failure.
func parseGateValue(t *testing.T, output, token string) string {
	t.Helper()

	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, token+":") {
			lines = append(lines, line)
		}
	}
	if len(lines) != 1 {
		t.Errorf("expected exactly one %q line in gate output, got %d — the gate's sed would return %s",
			token, len(lines), multiLineSummary(lines))
		if len(lines) == 0 {
			return ""
		}
	}

	re := regexp.MustCompile(regexp.QuoteMeta(token+": ") + `([0-9.]*)ms`)
	m := re.FindStringSubmatch(lines[len(lines)-1])
	if m == nil {
		t.Errorf("%q line does not match the gate's sed shape (Token: <n>ms): %q",
			token, lines[len(lines)-1])
		return ""
	}
	if m[1] == "" {
		t.Errorf("%s value missing before \"ms\" — the gate's sed would return an empty string and pass silently: %q",
			token, lines[len(lines)-1])
		return ""
	}
	v, parseErr := strconv.ParseFloat(m[1], 64)
	if parseErr != nil || v <= 0 {
		t.Errorf("%s value %q is not a positive number (bc would choke): %v", token, m[1], parseErr)
	}
	return m[1]
}

func multiLineSummary(lines []string) string {
	if len(lines) == 0 {
		return "nothing (empty parse, silently passing the threshold check)"
	}
	quoted := make([]string, len(lines))
	for i, l := range lines {
		quoted[i] = strconv.Quote(strings.TrimSpace(l))
	}
	return "a multi-line value: [" + strings.Join(quoted, ", ") + "]"
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source path")
	}
	// thisFile is the file itself: Dir once gets the package dir
	// (<module>/internal/localizer/fusion), three more gets <module>.
	pkg := filepath.Dir(thisFile)
	return filepath.Dir(filepath.Dir(filepath.Dir(pkg)))
}

// TestBenchmarkGuideDocumentsGate is the tripwire for the documentation half
// of the reconciliation (spaxel-fef904c5): the guide must keep describing
// the gate as-built. If the doc moves or stops naming the benchmark and the
// gate step, this fails before the guide can drift back into "add this
// step" / "already wired" self-contradiction.
func TestBenchmarkGuideDocumentsGate(t *testing.T) {
	// thisFile: <repo>/mothership/internal/localizer/fusion/<file> — Dir once
	// gets the package dir, four more gets the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source path")
	}
	pkg := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(pkg))))
	guide := filepath.Join(repoRoot, "docs", "ci-benchmark-integration.md")

	body, err := os.ReadFile(guide)
	if err != nil {
		t.Fatalf("reading docs/ci-benchmark-integration.md: %v", err)
	}
	for _, token := range []string{"BenchmarkFusionLoop", "timing-benchmark"} {
		if !strings.Contains(string(body), token) {
			t.Errorf("docs/ci-benchmark-integration.md no longer mentions %q — the guide and the gate have drifted apart", token)
		}
	}
}
