package privacy

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// This test enforces the no-cloud-relay guarantee by static inspection of the
// module source. It is fully deterministic and offline: it reads .go files
// under cmd/ and internal/ (everything that compiles into the shipped
// mothership binary) and never opens a network connection.
//
// Scope notes:
//   - _test.go sources are skipped: they are not part of the binary, and test
//     code legitimately uses httptest and loopback servers.
//   - Files with a `//go:build ignore` / `// +build ignore` constraint are
//     skipped: they are codegen/maintenance helpers (e.g. internal/oui's
//     registry downloader) that never compile into the binary.
//   - testdata/ directories are skipped.
//
// If this test fails on you: the mothership must not dial out from arbitrary
// code. Route the egress through internal/webhook (webhook.PostJSON) or the
// relevant user-configured integration package. Only add a package to
// egressAllowlist if its connections are genuinely directed by operator
// configuration — and explain why in the entry's comment.

// dialPatterns match Go constructs that open an outbound network connection.
// They are matched line-by-line against non-test module sources.
var dialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.Dial\(`),           // net.Dial, net.TCPConn.Dial-style helpers
	regexp.MustCompile(`DialTimeout\(`),      // net.DialTimeout
	regexp.MustCompile(`DialContext\(`),      // net.Dialer.DialContext, websocket.Dialer.DialContext
	regexp.MustCompile(`http\.Get\(`),        // convenience GET
	regexp.MustCompile(`http\.Post\(`),       // convenience POST
	regexp.MustCompile(`http\.Head\(`),       // convenience HEAD
	regexp.MustCompile(`http\.PostForm\(`),   // convenience form POST
	regexp.MustCompile(`http\.NewRequest\(`), // explicit request construction
	regexp.MustCompile(`http\.NewRequestWithContext\(`),
	regexp.MustCompile(`\.\s*Do\(`),         // http.Client.Do (any client variable)
	regexp.MustCompile(`mqtt\.NewClient\(`), // paho MQTT client — AddBroker dials inside the library
}

// suppressedPatterns are line shapes that textually match a dial pattern but
// are not network egress. Keep this list short and well-justified.
var suppressedPatterns = []*regexp.Regexp{
	// sync.Once.Do is lazy initialization, not a network dial.
	regexp.MustCompile(`\bonce\.Do\(`),
}

// egressAllowlist is the explicit set of packages allowed to contain outbound
// dial sites. Key: package directory, module-root-relative. Value: why that
// package's egress satisfies the no-cloud-relay guarantee.
//
// Every entry's connections target a destination the operator explicitly
// configured — a notification channel, a webhook/automation endpoint, the
// MQTT broker, or a diagnostic probe target. None of them receive detection
// data unless the operator wired that destination themselves.
var egressAllowlist = map[string]string{
	// Startup Phase 7 self-check: GET http://<bind>/healthz on its own
	// loopback listener to confirm the server came up.
	"cmd/mothership": "loopback self-check of its own /healthz listener during startup",

	// cmd/sim is a test fixture, not part of the deployed fleet: it dials the
	// operator's local mothership instance to ingest simulated data.
	"cmd/sim": "simulation fixture dialing the operator's local mothership instance",

	// Alert delivery to a user-configured alert receiver endpoint.
	"internal/analytics": "alert delivery to the operator-configured alert receiver URL",

	// Automation engine triggers user-configured HTTP actions.
	"internal/automation": "automation engine invoking operator-defined HTTP actions",

	// doctor probes MQTT (TCP) and NTP (UDP) endpoints from operator config
	// to diagnose fleet connectivity problems.
	"internal/doctor": "connectivity diagnostics probing operator-configured MQTT/NTP endpoints",

	// GitHub client looks up container releases for OTA updates; its BaseURL
	// is configuration, not a hard-coded cloud destination.
	"internal/github": "release lookup against the configured container-release API (BaseURL is operator config)",

	// The paho MQTT client connects to SPAXEL_MQTT_BROKER from operator
	// configuration; the TCP dial happens inside the library via AddBroker.
	"internal/mqtt": "MQTT transport to the operator-configured SPAXEL_MQTT_BROKER broker",

	// Notification channel integrations, all keyed on operator-configured
	// credentials/endpoints.
	"internal/notifications": "notification channels (ntfy, pushover, webhook) at operator-configured endpoints",
	"internal/notify":        "notification dispatch to operator-configured channel endpoints",

	// The shared webhook egress point: webhook.PostJSON plus the event
	// publisher. URLs always originate from trigger actions or channel
	// configuration. Detection-path code must route through here.
	"internal/webhook": "shared PostJSON/publisher egress point for operator-configured webhook URLs",
}

// detectionPathPackages are the packages that see raw CSI and person
// detections. They must contain zero outbound dial sites: detection data
// stays on the LAN and is only ever relayed by the operator-configured
// integration packages above.
var detectionPathPackages = []string{
	"internal/ingestion",
	"internal/fusion",
	"internal/api",
	"internal/dashboard",
	"internal/recorder",
	"internal/tracking",
}

// dialSite is one matched outbound-dial construct in module source.
type dialSite struct {
	file string // path relative to the module root, slash-separated
	line int    // 1-based
	text string // trimmed source line
}

func (s dialSite) String() string {
	return fmt.Sprintf("%s:%d: %s", s.file, s.line, s.text)
}

// moduleRoot locates the mothership module root (the directory containing
// go.mod) relative to this test file.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed; cannot locate module root")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("resolving module root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s does not contain go.mod: %v", root, err)
	}
	return root
}

// buildIgnored reports whether a Go source carries a `//go:build ignore` or
// `// +build ignore` constraint in its leading comment block (before the
// package clause). Such files never compile into the shipped binary.
func buildIgnored(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//") {
			return false // reached the package clause; no build constraint found
		}
		fields := strings.Fields(strings.TrimPrefix(trimmed, "//"))
		// "//go:build ignore" -> fields ["go:build", "ignore"]
		// "// +build ignore"  -> fields ["+build", "ignore"]
		if len(fields) == 2 && (fields[0] == "go:build" || fields[0] == "+build") && fields[1] == "ignore" {
			return true
		}
	}
	return false
}

// scanSources walks dir (module-root-relative), returning every dial site in
// non-test, non-build-ignored .go files, plus the number of files inspected.
// Results are sorted by file, then line, for deterministic output.
func scanSources(t *testing.T, root, dir string) (sites []dialSite, filesScanned int) {
	t.Helper()
	absDir := filepath.Join(root, dir)
	err := filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != absDir && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		filesScanned++
		if buildIgnored(src) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			matched := false
			for _, p := range dialPatterns {
				if p.MatchString(line) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			suppressed := false
			for _, p := range suppressedPatterns {
				if p.MatchString(line) {
					suppressed = true
					break
				}
			}
			if suppressed {
				continue
			}
			sites = append(sites, dialSite{
				file: filepath.ToSlash(rel),
				line: i + 1,
				text: strings.TrimSpace(line),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scanning %s: %v", dir, err)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].file != sites[j].file {
			return sites[i].file < sites[j].file
		}
		return sites[i].line < sites[j].line
	})
	return sites, filesScanned
}

// TestEgressDialSitesAreAllowlisted is the core no-cloud-relay guard: every
// outbound dial construct in the module must live in a package on the
// explicit egress allowlist. A new dial site anywhere else fails here with a
// message naming the offending file and line.
func TestEgressDialSitesAreAllowlisted(t *testing.T) {
	root := moduleRoot(t)

	var sites []dialSite
	filesScanned := 0
	for _, dir := range []string{"cmd", "internal"} {
		s, n := scanSources(t, root, dir)
		sites = append(sites, s...)
		filesScanned += n
	}

	// Sanity guards so structural changes (moved module root, a broken
	// scanner) can never turn this into a silent pass.
	if filesScanned < 100 {
		t.Fatalf("sanity: only %d non-test .go files scanned under cmd/ + internal/; expected a module of 100+ — the scan scope is broken", filesScanned)
	}
	if len(sites) == 0 {
		t.Fatal("sanity: scanner found zero dial sites; the patterns or scope are broken")
	}

	var offending []string
	for _, s := range sites {
		pkg := path.Dir(s.file)
		if _, ok := egressAllowlist[pkg]; !ok {
			offending = append(offending, s.String())
		}
	}
	if len(offending) > 0 {
		t.Errorf("found %d outbound dial site(s) outside the egress allowlist — a no-cloud-relay violation:\n  %s\n\nThe mothership must not open connections from arbitrary code. Route the egress through internal/webhook (webhook.PostJSON) or another operator-configured integration package. Only if the package's egress is genuinely operator-directed may you add it to egressAllowlist, with a justification comment.",
			len(offending), strings.Join(offending, "\n  "))
	}
}

// TestDetectionPathHasNoDialSites asserts the stronger property for the
// packages that handle raw CSI and person detections: zero outbound dial
// sites at all, not merely allowlisted ones.
func TestDetectionPathHasNoDialSites(t *testing.T) {
	root := moduleRoot(t)

	for _, pkg := range detectionPathPackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			dir := filepath.Join(root, pkg)
			if _, err := os.Stat(dir); err != nil {
				t.Fatalf("detection-path package %s is missing from the module: %v (update detectionPathPackages)", pkg, err)
			}
			sites, filesScanned := scanSources(t, root, pkg)
			if filesScanned == 0 {
				t.Fatalf("sanity: 0 non-test .go files scanned in %s; scan scope is broken", pkg)
			}
			if len(sites) > 0 {
				msgs := make([]string, len(sites))
				for i, s := range sites {
					msgs[i] = s.String()
				}
				t.Errorf("detection-path package %s must contain no outbound dial sites, found %d:\n  %s",
					pkg, len(sites), strings.Join(msgs, "\n  "))
			}
		})
	}
}

// TestEgressAllowlistHasNoStaleEntries keeps the allowlist honest: every
// entry must name an existing package that still contains at least one dial
// site. When the last dial leaves a package, the entry must be removed — the
// allowlist documents where egress lives today, not where it used to.
func TestEgressAllowlistHasNoStaleEntries(t *testing.T) {
	root := moduleRoot(t)

	pkgs := make([]string, 0, len(egressAllowlist))
	for pkg := range egressAllowlist {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)

	for _, pkg := range pkgs {
		dir := filepath.Join(root, pkg)
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("egressAllowlist entry %q: package directory missing: %v", pkg, err)
			continue
		}
		sites, _ := scanSources(t, root, pkg)
		if len(sites) == 0 {
			t.Errorf("egressAllowlist entry %q is stale: that package no longer contains any dial sites; remove the entry", pkg)
		}
	}
}
