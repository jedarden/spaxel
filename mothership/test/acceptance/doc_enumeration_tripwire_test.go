// Doc tripwire for the acceptance-suite enumeration (spaxel-d2fb760a).
//
// README.md and docs/codebase-structure-and-test-patterns.md both enumerate
// the AS scenarios, and both silently understated the suite as "AS-1…AS-7"
// after AS-8 (2D position accuracy) and AS-9 (person count) landed. This test
// derives the implemented scenario set from the as*_test.go files actually
// present in this directory and requires each document's canonical
// "AS-1 … AS-N" enumeration to match that set exactly — so the enumeration
// cannot go stale again. Same pattern as the timing-gate doc tripwire
// (TestBenchmarkGuideDocumentsGate in internal/localizer/fusion, from
// spaxel-fef904c5).
//
// AS-10 (stationary/breathing) is assigned in
// docs/notes/localization-capability-acceptance-map.md and its simulator
// fixture has landed (cmd/sim/breathing.go, --scenario stationary) but has no
// acceptance test file yet, so the enumeration says AS-9. When
// as10_*_test.go lands, this test fails until both documents say AS-10 —
// that is the tripwire working, not a bug in it.
//
// This test is static (file parsing only): it needs no built binaries and
// runs in the plain `go test ./...` pass, unlike the scenario tests, which
// gate on SPAXEL_INTEGRATION_TEST=1.
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

// asNumberRe extracts the scenario number from an as<N>_*_test.go filename.
var asNumberRe = regexp.MustCompile(`^as(\d+)_.*_test\.go$`)

// enumerationRe matches the canonical enumeration phrase "AS-1 … AS-N"
// (Unicode ellipsis or three ASCII dots, any spacing). Anchoring at AS-1 is
// deliberate: prose references to individual scenarios ("AS-10 is assigned",
// "see AS-3") must not be misread as the implemented enumeration, so keep
// exactly one range phrase per document.
var enumerationRe = regexp.MustCompile(`AS-1\s*(?:…|\.\.\.)\s*AS-(\d+)`)

// mapRef is the acceptance-map document both enumerating docs must reference.
const mapRef = "localization-capability-acceptance-map.md"

func TestAcceptanceDocsEnumerateImplementedScenarios(t *testing.T) {
	// thisFile: <repo>/mothership/test/acceptance/<file> — Dir once gets this
	// package dir, three more gets the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine source path")
	}
	pkgDir := filepath.Dir(thisFile)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(pkgDir)))

	// Implemented set: every as<N>_*_test.go file in this directory. The
	// pre-existing AS-5 double-use (as5_ota_test.go and
	// as5_wifi_restart_race_test.go) collapses naturally under set semantics.
	files, err := filepath.Glob(filepath.Join(pkgDir, "as*_test.go"))
	if err != nil {
		t.Fatalf("globbing as*_test.go: %v", err)
	}
	implemented := map[int]bool{}
	for _, f := range files {
		base := filepath.Base(f)
		m := asNumberRe.FindStringSubmatch(base)
		if m == nil {
			t.Fatalf("%s matches the as*_test.go glob but not the as<N>_*_test.go naming convention — rename it or widen asNumberRe", base)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("parsing scenario number from %s: %v", base, err)
		}
		implemented[n] = true
	}
	if len(implemented) == 0 {
		t.Fatal("no as*_test.go files found — the acceptance suite is missing from this checkout")
	}

	for _, doc := range []struct{ rel string }{
		{"README.md"},
		{filepath.Join("docs", "codebase-structure-and-test-patterns.md")},
	} {
		t.Run(doc.rel, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repoRoot, doc.rel))
			if err != nil {
				t.Fatalf("reading %s: %v", doc.rel, err)
			}
			text := string(body)

			if !strings.Contains(text, mapRef) {
				t.Errorf("%s no longer references docs/notes/%s — the map is the scenario-numbering and threshold authority; restore the cross-reference", doc.rel, mapRef)
			}

			matches := enumerationRe.FindAllStringSubmatch(text, -1)
			if len(matches) != 1 {
				t.Errorf("%s must carry exactly one canonical \"AS-1 … AS-N\" acceptance-suite enumeration; found %d — keep a single range phrase so the tripwire can parse it", doc.rel, len(matches))
				return
			}
			n, err := strconv.Atoi(matches[0][1])
			if err != nil {
				t.Fatalf("parsing enumeration bound in %s: %v", doc.rel, err)
			}
			enumerated := map[int]bool{}
			for i := 1; i <= n; i++ {
				enumerated[i] = true
			}

			var unlisted, phantom []int
			for id := range implemented {
				if !enumerated[id] {
					unlisted = append(unlisted, id)
				}
			}
			for id := range enumerated {
				if !implemented[id] {
					phantom = append(phantom, id)
				}
			}
			sort.Ints(unlisted)
			sort.Ints(phantom)
			if len(unlisted) > 0 || len(phantom) > 0 {
				t.Errorf("%s enumerates AS-1…AS-%d but mothership/test/acceptance/ contains scenarios %v — the enumeration has drifted from the suite (unlisted test files: %v; enumerated without a test file: %v). Update the enumeration to the implemented set; AS-10 status lives in docs/notes/%s",
					doc.rel, n, sortedInts(implemented), unlisted, phantom, mapRef)
			}
		})
	}
}

// sortedInts returns the keys of a int-set in ascending order, for messages.
func sortedInts(set map[int]bool) []int {
	keys := make([]int, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
