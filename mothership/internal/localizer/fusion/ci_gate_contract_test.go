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
// The template side was hardened 2026-09-25 (declarative-config 5d7987b9,
// bead spaxel-d8268220, documented in docs/ci-benchmark-integration.md): the
// step now runs bash with pipefail and enforces the exactly-one-Median/one-P99
// invariant plus a numeric parse guard before any threshold comparison, so a
// broken or absent benchmark fails the timing node itself. This file remains
// the in-repo half of the contract: it keeps the benchmark's output shape
// gate-compatible and fails the go-test leg on drift, independent of what
// the template does.
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
	"time"
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
// "gate ineffective" shapes: each made the step's old comparison a silent
// no-op instead of a threshold failure (the template now guards all three,
// but this test keeps the output shape contract enforced from this side too).
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

// repoRoot is the spaxel checkout containing this module — one Dir above
// moduleRoot. Used by the doc tripwires, which read docs/ from the repo.
func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(moduleRoot(t))
}

// TestBenchmarkGuideDocumentsGate is the tripwire for the documentation half
// of the reconciliation (spaxel-fef904c5): the guide must keep describing
// the gate as-built. If the doc moves or stops naming the benchmark and the
// gate step, this fails before the guide can drift back into "add this
// step" / "already wired" self-contradiction.
func TestBenchmarkGuideDocumentsGate(t *testing.T) {
	guide := filepath.Join(repoRoot(t), "docs", "ci-benchmark-integration.md")

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

// thresholdMarkerRe matches the doc's canonical single-sourcing marker —
// the one machine-readable record both sides of the threshold convention
// reconcile against (see TestThresholdsSingleSourced and
// scripts/check-fusion-timing-thresholds.sh, which parse the same shape).
var thresholdMarkerRe = regexp.MustCompile(
	`spaxel-fusion-timing-thresholds:\s+production_ms=(\d+)\s+ci_ms=(\d+)\s+hard_ms=(\d+)`)

// TestThresholdsSingleSourced is the repo-side half of the threshold
// reconciliation (spaxel-37fa27a9): the doc's canonical marker must equal
// the constants the tests actually enforce, and the doc's embedded copy of
// the gate script must carry the same ci_threshold=/hard_limit= values.
// Without it, a one-sided change — new constant, stale marker, or an
// updated script quote — drifted silently until "a review" noticed.
// Deliberately not skipped in -short mode: it is file parsing, not timing.
// The template-vs-repo half (live ci_threshold/hard_limit vs marker) is
// scripts/check-fusion-timing-thresholds.sh — the template lives in
// declarative-config, outside this repo's reach at test time.
func TestThresholdsSingleSourced(t *testing.T) {
	guide := filepath.Join(repoRoot(t), "docs", "ci-benchmark-integration.md")
	body, err := os.ReadFile(guide)
	if err != nil {
		t.Fatalf("reading docs/ci-benchmark-integration.md: %v", err)
	}

	matches := thresholdMarkerRe.FindAllStringSubmatch(string(body), -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one canonical threshold marker line ('spaxel-fusion-timing-thresholds: production_ms=<n> ci_ms=<n> hard_ms=<n>') in docs/ci-benchmark-integration.md, found %d — the single-sourcing convention itself is broken", len(matches))
	}
	m := matches[0]

	for _, c := range []struct {
		name string
		want time.Duration
		got  string
	}{
		{"production_ms", productionTarget, m[1]},
		{"ci_ms", ciThreshold, m[2]},
		{"hard_ms", hardLimit, m[3]},
	} {
		n, parseErr := strconv.Atoi(c.got)
		if parseErr != nil {
			t.Fatalf("marker field %s=%q does not parse as an integer: %v", c.name, c.got, parseErr)
		}
		got := time.Duration(n) * time.Millisecond
		if got != c.want {
			t.Errorf("threshold marker %s=%d (%v) disagrees with timing_budget_test.go's enforced constant %v — update the marker AND the constants together, then run scripts/check-fusion-timing-thresholds.sh against the live template",
				c.name, n, got, c.want)
		}
	}

	// The doc's embedded copy of the gate script is this repo's record of the
	// template side; it must quote the same values the marker carries.
	for _, c := range []struct {
		token string
		ms    string
	}{
		{"ci_threshold", m[2]},
		{"hard_limit", m[3]},
	} {
		want := c.token + "=" + c.ms
		if !strings.Contains(string(body), want) {
			t.Errorf("docs/ci-benchmark-integration.md's embedded gate script no longer quotes %q — the quoted script and the marker have drifted apart in the same doc; update the quote to match", want)
		}
	}
}
