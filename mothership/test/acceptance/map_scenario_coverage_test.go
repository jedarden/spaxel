// Coverage guard tying every acceptance-map scenario to a test file
// (spaxel-103c62e8).
//
// docs/notes/localization-capability-acceptance-map.md is the numbering and
// threshold authority for the whole suite (§2 rules), and §4 assigns every
// README capability claim a scenario ID and a target file — but nothing
// failed when an assignment sat without its test file. AS-10 was assigned on
// 2026-09-25 and stayed unimplemented silently, discoverable only by reading
// the map's status header and the README /
// docs/codebase-structure-and-test-patterns.md footnotes. This test derives
// the assigned scenario set from the map itself and requires every assigned
// number to resolve to a test file in this directory:
//
//   - the canonical as<N>_*_test.go, or
//   - a rehomed home listed in rehomedTestFiles below (a test deliberately
//     living outside the asN_ namespace, documented in the map), or
//   - an explicit pending/waived marker on a map line that names the
//     scenario — the map saying "acceptance test pending" in so many words,
//     as it does today for AS-10.
//
// An assigned scenario with no file and no marker is a red test: "assigned
// but unimplemented" stops being a documentation note. The shape mirrors the
// dashboard a11y entrypoint guard
// (dashboard/tests/a11y-entrypoint-coverage.spec.js), which fails when a new
// dashboard/*.html entry point ships without a spec page.
//
// Static (file parsing only): needs no built binaries and runs in the plain
// `go test ./...` pass, like the enumeration tripwire
// (TestAcceptanceDocsEnumerateImplementedScenarios), which enforces the
// opposite direction (implemented files → doc enumerations). File presence
// only: no scenario is run and no scenario's pass/fail changes.
package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// scenarioIDRowRe matches a §4 capability-section assignment row: e.g.
// "| Scenario ID | **AS-8** |" or "| Scenario ID | **AS-2-ext** (no new
// number …)". The optional "-ext" marks an extend-in-place scenario, which
// resolves to its base number's file (AS-2-ext lives in as2_*_test.go).
var scenarioIDRowRe = regexp.MustCompile(`\|\s*Scenario ID\s*\|\s*\*\*AS-(\d+)(?:-ext)?\*\*`)

// asFileTokenRe matches a backticked `as<N>_<name>_test.go` token anywhere in
// the map — the §2 suite-inventory rows and the §2 "This map assigns" bullets
// both name their scenario's file this way, so an assignment made in either
// shape is picked up. Requiring the full backticked file name keeps bare
// prose mentions ("rule 4 blocks reserving AS-11") from creating phantom
// assignments.
var asFileTokenRe = regexp.MustCompile("`as(\\d+)_[a-z0-9_]+_test\\.go`")

// mapPendingMarkers are the phrasings that count as the map explicitly
// declaring "this scenario deliberately has no test file yet". A map line
// naming the scenario (an AS-N token or an asN_ file token) that also carries
// one of these substrings is an explicit pending/waived marker. Over-matching
// is harmless — markers are consulted only for numbers without a test file —
// while under-matching would produce a false red.
var mapPendingMarkers = []string{
	"pending",         // status header: "acceptance test pending"
	"not yet written", // §8: "acceptance test not yet written"
	"not validated",   // §8 recorded-status vocabulary
	"unimplemented",   // "assigned-but-unimplemented"
	"waived",
	"deferred",
	"blocked",
}

// rehomedTestFiles lets a scenario's test live outside the as<N>_ namespace
// when the map documents the rehoming — the mirror image of §2 rule 2, which
// keeps non-scenario tests out of the namespace. Empty today: every assigned
// scenario resolves canonically, and the one historical rehoming went the
// other way — wifi_restart_race_test.go was moved *out* of the AS-5 number
// (spaxel-1cd1155f), it is not AS-5's home. If a scenario is ever rehomed,
// add its number here with a comment citing the map section that documents
// it; the map row is the authority, this table is the guard's mirror of it.
var rehomedTestFiles = map[int]string{}

func TestAcceptanceMapScenariosResolveToTestFiles(t *testing.T) {
	// thisFile: <repo>/mothership/test/acceptance/<file> — Dir once gets this
	// package dir, three more gets the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source path")
	}
	pkgDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(pkgDir)))

	body, err := os.ReadFile(filepath.Join(repoRoot, "docs", "notes", mapRef))
	if err != nil {
		t.Fatalf("reading acceptance map: %v", err)
	}
	text := string(body)
	lines := strings.Split(text, "\n")

	// Assigned set: every scenario number the map assigns, via a §4 Scenario
	// ID row or a backticked asN_ file token. "-ext" scenarios reduce to
	// their base number — the extension lives in the base file.
	assigned := map[int]bool{}
	for _, m := range scenarioIDRowRe.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parsing scenario number from %q: %v", m[0], err)
		}
		assigned[n] = true
	}
	for _, m := range asFileTokenRe.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parsing scenario number from %q: %v", m[0], err)
		}
		assigned[n] = true
	}
	if len(assigned) == 0 {
		t.Fatal("no scenario assignments found in the map — scenarioIDRowRe / asFileTokenRe no longer match the map's assignment shape and the guard has gone blind")
	}

	// Implemented set: the as<N>_*_test.go files in this directory. The
	// naming-convention check belongs to the enumeration tripwire; a file it
	// will fail on is skipped here rather than double-reported.
	files, err := filepath.Glob(filepath.Join(pkgDir, "as*_test.go"))
	if err != nil {
		t.Fatalf("globbing as*_test.go: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no as*_test.go files found — the acceptance suite is missing from this checkout")
	}
	implemented := map[int]bool{}
	for _, f := range files {
		if m := asNumberRe.FindStringSubmatch(filepath.Base(f)); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("parsing scenario number from %s: %v", f, err)
			}
			implemented[n] = true
		}
	}

	// A rehomed home is only a resolution if the file actually exists.
	for n, f := range rehomedTestFiles {
		if _, err := os.Stat(filepath.Join(pkgDir, f)); err != nil {
			t.Errorf("rehomedTestFiles[AS-%d] = %q but the file is missing — restore it, or update the map and this table together", n, f)
		}
	}

	// Every assigned number must resolve: canonical file, rehomed home, or
	// an explicit pending/waived marker in the map.
	var unimplemented []int
	for n := range assigned {
		switch {
		case implemented[n]:
			// covered
		case rehomedTestFiles[n] != "":
			// covered (existence checked above)
		default:
			if line, ok := findMapPendingMarker(lines, n); ok {
				t.Logf("AS-%d is assigned but has no test file; the map carries the explicit pending marker at line %d: %s",
					n, line, strings.TrimSpace(lines[line-1]))
			} else {
				unimplemented = append(unimplemented, n)
			}
		}
	}
	sort.Ints(unimplemented)
	if len(unimplemented) > 0 {
		t.Errorf("the map assigns AS scenarios %v but neither an as<N>_*_test.go file nor a rehomed home exists for %v, and no map line naming them carries an explicit pending/waived marker — write the test file, record the rehoming (map + rehomedTestFiles), or mark the scenario pending/waived in docs/notes/%s (§2 numbering rules); \"scenario assigned but unimplemented\" must not be silent",
			sortedInts(assigned), unimplemented, mapRef)
	}

	// Inverse direction, §2 rule 1 (the map is the numbering authority): an
	// as<N> file may only claim a number the map assigns.
	var unassigned []int
	for n := range implemented {
		if !assigned[n] {
			unassigned = append(unassigned, n)
		}
	}
	sort.Ints(unassigned)
	if len(unassigned) > 0 {
		t.Errorf("%v claim scenario numbers the map does not assign — docs/notes/%s is the numbering authority (§2 rule 1); add the assignment to the map or rename the file out of the as<N>_ namespace (wifi_restart_race_test.go precedent, spaxel-1cd1155f)",
			unassigned, mapRef)
	}
}

// findMapPendingMarker reports whether any map line naming scenario n — by an
// AS-N token or an asN_ file token — also carries an explicit pending/waived
// marker, returning the 1-based line number for messages. The trailing
// underscore in the file-token form ("as1_") keeps AS-1 from matching inside
// an as10_ file name, and the \b in the AS-N form keeps AS-1 from matching
// inside AS-10.
func findMapPendingMarker(lines []string, n int) (int, bool) {
	num := strconv.Itoa(n)
	asToken := regexp.MustCompile(`AS-` + num + `\b`)
	fileToken := "as" + num + "_"
	for i, line := range lines {
		lower := strings.ToLower(line)
		if !asToken.MatchString(line) && !strings.Contains(lower, fileToken) {
			continue
		}
		for _, marker := range mapPendingMarkers {
			if strings.Contains(lower, marker) {
				return i + 1, true
			}
		}
	}
	return 0, false
}
