package privacy

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This test enforces the prohibited-media guarantee: Spaxel senses its
// environment through WiFi CSI alone and must contain no camera or
// microphone capture path. It is the media sibling of
// egress_allowlist_test.go (no-cloud-relay) and equally deterministic and
// offline — it only reads source files.
//
// The surfaces that can physically touch a camera or microphone are the
// browser (the dashboard — a getUserMedia call is the one way a web page
// captures media) and the sensor firmware (an ESP32-S3 node with an
// esp32-camera driver or an I2S/PDM digital microphone). The mothership
// server has no media hardware and its data-flow restrictions are already
// covered by the egress tests; both real surfaces are scanned here:
//
//   - dashboard/  — first-party .html/.js/.mjs/.css (node_modules pruned:
//     it is git-ignored and absent from a clean checkout, so the scan must
//     not depend on it)
//   - firmware/   — first-party C/C++ and sdkconfig/Kconfig configuration
//     (build/ is generated output; managed_components/ is vendored third
//     party whose chip-capability macros — e.g. the ESP32-S3's
//     CONFIG_SOC_I2S_SUPPORTS_LCD_CAMERA — describe silicon features, not
//     application behavior)
//
// Deliberate non-goals, documented so they are not "fixed" later:
//   - AudioContext / new Audio() playback is allowed and must stay allowed:
//     the dashboard plays audible alerts (dashboard/js/anomaly.js,
//     dashboard/js/security-panel.js). The guarantee bans capture, not
//     sound output — which is why no pattern below matches playback APIs.
//   - canvas.captureStream() records on-page canvas content, not a physical
//     sensor, and is not matched.
//   - Generic ADC helpers (adc_oneshot/adc_continuous) are not matched: an
//     ADC is not a microphone, and battery/analog telemetry is legitimate.
//     An actual analog-microphone path would still be a reviewable defect
//     under this file's contract even though no pattern names it.
//   - docs/ and dashboard/*.json|.md prose may name these APIs when
//     documenting the prohibition itself; only code and configuration are
//     scanned.
//
// If this test fails on you: Spaxel is CSI-only by design and there is no
// allowlist to join. Remove the capture path (and the hardware dependency
// it implies) rather than weakening the pattern list. The one sanctioned
// escape hatch is renaming the invariant itself, which is a project-level
// decision to be made in docs/notes/network-boundary.md, not in code
// review of a single change.

// browserCapturePatterns match the W3C media-capture APIs. Case-sensitive:
// the platform APIs are camelCase and the near-miss lowercase forms (e.g. a
// CSS class named "mediadevices") are not API references.
var browserCapturePatterns = []*regexp.Regexp{
	regexp.MustCompile(`getUserMedia`),     // navigator.mediaDevices.getUserMedia / legacy navigator.getUserMedia — camera+mic capture
	regexp.MustCompile(`mediaDevices`),     // the navigator.mediaDevices entry point and every method on it
	regexp.MustCompile(`getDisplayMedia`),  // screen/window capture
	regexp.MustCompile(`enumerateDevices`), // camera/mic device enumeration — the recon step for a capture path
	regexp.MustCompile(`MediaRecorder`),    // encoding captured media for exfiltration
	regexp.MustCompile(`MediaStreamTrack`), // direct track plumbing for captured device streams
	regexp.MustCompile(`ImageCapture`),     // still-photo capture API
}

// firmwareCapturePatterns match the ESP-IDF capture drivers. Case-sensitive:
// IDF APIs are lowercase or Kconfig-uppercase, and matching bare `I2S` or
// `camera` would false-positive on chip-capability macros like
// CONFIG_SOC_I2S_SUPPORTS_LCD_CAMERA elsewhere in the tree.
var firmwareCapturePatterns = []*regexp.Regexp{
	regexp.MustCompile(`esp_camera`),          // the esp32-camera driver (esp_camera_init, esp_camera.h, camera_config_t)
	regexp.MustCompile(`CONFIG_ESP32_CAMERA`), // the esp32-camera component's Kconfig knob
	regexp.MustCompile(`i2s_driver_install`),  // legacy I2S peripheral bring-up (digital/PDM mic transport)
	regexp.MustCompile(`i2s_new_channel`),     // current I2S peripheral bring-up
	regexp.MustCompile(`i2s_channel_read`),    // current-API I2S sample reads
	regexp.MustCompile(`i2s_read`),            // legacy-API I2S sample reads
	regexp.MustCompile(`i2s_pdm_rx`),          // PDM microphone configuration
	regexp.MustCompile(`I2S_STD_CAPTURE\b`),   // (deprecated std-capture slot naming; kept for older IDF vendoring)
}

// mediaScanScope describes one first-party tree to walk.
type mediaScanScope struct {
	dir        string          // repo-root-relative directory to walk
	extensions map[string]bool // files kept, by lowercase extension
	bareNames  map[string]bool // extensionless filenames kept (sdkconfig)
	skipDirs   map[string]bool // directory names pruned during the walk
}

// dashboardScope is every first-party browser asset the shipped dashboard
// loads. Vendored-but-tracked bundles (agentation.js, static/vendor/) stay
// in scope: they execute in the page and must obey the same guarantee.
var dashboardScope = mediaScanScope{
	dir:        "dashboard",
	extensions: setOf(".html", ".js", ".mjs", ".css"),
	skipDirs:   setOf("node_modules"),
}

// firmwareScope is the first-party firmware source and its configuration.
var firmwareScope = mediaScanScope{
	dir:        "firmware",
	extensions: setOf(".c", ".h", ".cpp", ".hpp", ".txt", ".in", ".projbuild", ".defaults"),
	bareNames:  setOf("sdkconfig"),
	skipDirs:   setOf("build", "managed_components"),
}

// setOf builds a string set from literals.
func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// repoRoot locates the repository root: the parent of the mothership module
// root. The dashboard and firmware trees are siblings of the module, so
// every scope path is asserted to exist before scanning.
func repoRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(moduleRoot(t), "..")
	for _, must := range []string{"dashboard", "firmware", filepath.Join("mothership", "go.mod")} {
		if _, err := os.Stat(filepath.Join(root, must)); err != nil {
			t.Fatalf("repo root %s is missing %s: %v", root, must, err)
		}
	}
	return root
}

// scan walks the scope under root, returning every line matching any of the
// patterns plus the number of files inspected. Results are sorted by file,
// then line, for deterministic output. Reuses dialSite (file/line/text) from
// egress_allowlist_test.go.
func (sc mediaScanScope) scan(t *testing.T, root string, patterns []*regexp.Regexp) (sites []dialSite, filesScanned int) {
	t.Helper()
	absDir := filepath.Join(root, sc.dir)
	err := filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != absDir && sc.skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == "" {
			if !sc.bareNames[name] {
				return nil
			}
		} else if !sc.extensions[ext] {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		filesScanned++
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			for _, p := range patterns {
				if p.MatchString(line) {
					sites = append(sites, dialSite{
						file: filepath.ToSlash(rel),
						line: i + 1,
						text: strings.TrimSpace(line),
					})
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scanning %s: %v", sc.dir, err)
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].file != sites[j].file {
			return sites[i].file < sites[j].file
		}
		return sites[i].line < sites[j].line
	})
	return sites, filesScanned
}

// TestDashboardHasNoMediaCaptureSites asserts the browser half of the
// prohibited-media guarantee: no first-party dashboard asset references a
// camera, microphone or screen-capture API.
func TestDashboardHasNoMediaCaptureSites(t *testing.T) {
	root := repoRoot(t)

	sites, filesScanned := dashboardScope.scan(t, root, browserCapturePatterns)

	// Sanity guards so a moved tree or broken extension filter can never
	// turn this into a silent pass over a handful of files.
	if filesScanned < 100 {
		t.Fatalf("sanity: only %d dashboard assets scanned (expected 100+: html+js+css excluding node_modules); the scan scope is broken", filesScanned)
	}
	htmlFiles := countExtension(t, root, dashboardScope, ".html")
	if htmlFiles < 8 {
		t.Fatalf("sanity: only %d dashboard .html files scanned (expected 8+); the scan scope is broken", htmlFiles)
	}

	if len(sites) > 0 {
		msgs := make([]string, len(sites))
		for i, s := range sites {
			msgs[i] = s.String()
		}
		t.Errorf("found %d media-capture reference(s) in dashboard source — Spaxel is CSI-only; camera, microphone and screen capture are prohibited:\n  %s\n\nRemove the capture path; do not weaken the pattern list (see this file's header).",
			len(sites), strings.Join(msgs, "\n  "))
	}
}

// TestFirmwareHasNoCaptureDriverSites asserts the sensor half of the
// prohibited-media guarantee: no first-party firmware source or
// configuration references the esp32-camera driver or an I2S/PDM digital
// microphone path.
func TestFirmwareHasNoCaptureDriverSites(t *testing.T) {
	root := repoRoot(t)

	sites, filesScanned := firmwareScope.scan(t, root, firmwareCapturePatterns)

	if filesScanned < 40 {
		t.Fatalf("sanity: only %d firmware source/config files scanned (expected 40+); the scan scope is broken", filesScanned)
	}

	if len(sites) > 0 {
		msgs := make([]string, len(sites))
		for i, s := range sites {
			msgs[i] = s.String()
		}
		t.Errorf("found %d capture-driver reference(s) in firmware source/config — Spaxel nodes sense CSI only; cameras and I2S/PDM microphones are prohibited:\n  %s\n\nRemove the capture path; do not weaken the pattern list (see this file's header).",
			len(sites), strings.Join(msgs, "\n  "))
	}
}

// countExtension counts scope files carrying one extension — a finer sanity
// probe than the total, so a filter regression that silently drops .html
// still fails loudly.
func countExtension(t *testing.T, root string, sc mediaScanScope, ext string) int {
	t.Helper()
	count := 0
	absDir := filepath.Join(root, sc.dir)
	err := filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != absDir && sc.skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) == ext {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("counting %s files in %s: %v", ext, sc.dir, err)
	}
	return count
}
