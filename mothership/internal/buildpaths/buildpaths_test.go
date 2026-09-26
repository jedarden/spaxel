// Package buildpaths is a test-only regression gate for the spaxel
// build-path trigger contract — specifically, that managed ESP-IDF component
// changes trigger firmware/image builds, and that the core tree classes
// (mothership source, dashboard runtime assets, root build configuration)
// keep triggering while the ignore list stays doc/bookkeeping-only.
//
// Background (spaxel-1b0e96aa): docs/SYSTEM_CATALOG.md (§Build Impact
// Classification) lists firmware/managed_components/ as a firmware build
// trigger, and it is right about relevance — the vendored
// espressif__esp_websocket_client and espressif__mdns trees are compiled into
// spaxel.bin, so their content ships in every published image. The
// consolidated specification (docs/build-path-filter-spec.md) recorded only
// that the directory is gitignored, which reads as "not a build input" and
// left the managed-component trigger set implicit.
//
// The reconciliation this package pins:
//
//  1. The directory content is gitignored, so a push can never carry a direct
//     change under firmware/managed_components/. Managed-component changes
//     reach the repository — and therefore CI — only through two tracked
//     carrier files: firmware/main/idf_component.yml (the component-manager
//     manifest) and firmware/dependencies.lock (the resolved-version pin, the
//     firmware analogue of go.sum). Both are Tier A build triggers.
//  2. The live sensor filter (jedarden/declarative-config →
//     k8s/iad-ci/argo-events/spaxel-sensor.yml) is exclusion-based: a push
//     builds unless every changed path in every commit is ignored. The
//     predicate below mirrors the documented live ignore list
//     (build-path-filter-spec.md §4.1); the spec's §5.1 additions are
//     proposals, not part of this contract.
//  3. If the managed_components directory is ever vendored (the gitignore
//     rule dropped), its paths become direct Tier A triggers — the spec's
//     §5.2 explicit trigger regex names firmware/managed_components/ so the
//     explicit form cannot regress either.
//
// A trigger fires spaxel-build, whose firmware-build leg recompiles the
// managed components into spaxel-firmware-${VERSION}.bin and whose
// docker-build leg bakes that artifact into the published image — so
// "triggers" throughout this package means both the firmware and image
// builds.
package buildpaths

import (
	"os"
	"strings"
	"testing"
)

// ignoredByLiveFilter mirrors the live spaxel-sensor ignore list, as
// documented in docs/build-path-filter-spec.md §4.1 ("already excluded by the
// live filter") and docs/notes/ci-doc-only-push-path-filter.md. If the sensor
// ignore list in declarative-config changes, this predicate must be updated
// in lockstep — it is the in-repo record of the trigger contract.
func ignoredByLiveFilter(path string) bool {
	switch {
	case strings.HasPrefix(path, "docs/"),
		strings.HasPrefix(path, ".beads/"),
		strings.HasPrefix(path, ".needle"),
		strings.HasSuffix(path, ".md"),
		path == "LICENSE",
		path == ".gitignore":
		return true
	}
	return false
}

// triggersBuild reports whether a changed path makes the spaxel-sensor fire
// the spaxel-build workflow (firmware build + image build). The sensor is
// exclusion-based: anything not ignored triggers.
func triggersBuild(path string) bool {
	return !ignoredByLiveFilter(path)
}

// pushTriggersBuild mirrors the sensor's conjunctive skip rule: a push is
// skipped only when every changed path in every commit matches an ignored
// pattern. Any single substantive path — e.g. a managed-component pin bump —
// builds, even when it shares the push with bookkeeping churn.
func pushTriggersBuild(paths ...string) bool {
	for _, p := range paths {
		if triggersBuild(p) {
			return true
		}
	}
	return false
}

// TestManagedComponentChangesTriggerBuild proves that every path through
// which a managed-component change can reach a push classifies as a
// build-triggering path.
func TestManagedComponentChangesTriggerBuild(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		// Tracked carriers of managed-component changes (Tier A).
		{
			name: "component manifest declares managed components",
			path: "firmware/main/idf_component.yml",
			want: true,
		},
		{
			name: "component lockfile pins resolved versions",
			path: "firmware/dependencies.lock",
			want: true,
		},
		// Vendored component paths are gitignored today, so no push can carry
		// them — but if the directory is ever vendored (the ignore rule
		// dropped), these are direct build triggers and must stay so.
		{
			name: "vendored esp_websocket_client source",
			path: "firmware/managed_components/espressif__esp_websocket_client/esp_websocket_client.c",
			want: true,
		},
		{
			name: "vendored mdns header",
			path: "firmware/managed_components/espressif__mdns/include/mdns.h",
			want: true,
		},
		// Controls: firmware source triggers; documented ignore rules do not.
		{
			name: "firmware source control",
			path: "firmware/main/main.c",
			want: true,
		},
		{
			name: "docs markdown control",
			path: "docs/notes/some-note.md",
			want: false,
		},
		{
			name: "bead checkpoint control",
			path: ".beads/checkpoint/current.json",
			want: false,
		},
		{
			name: "root markdown control",
			path: "README.md",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triggersBuild(tt.path); got != tt.want {
				t.Errorf("triggersBuild(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestMixedPushWithManagedComponentChangeTriggersBuild pins the conjunctive
// skip rule for the managed-component carriers: bead-churn or docs churn that
// shares a push with a component pin bump must still build. Loosening the
// rule to "any ignored path skips" would let a component upgrade hide behind
// a checkpoint graft in the same push.
func TestMixedPushWithManagedComponentChangeTriggersBuild(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{
			name:  "checkpoint graft alone does not build",
			paths: []string{".beads/checkpoint/current.json", ".beads/checkpoint/forensic.jsonl"},
			want:  false,
		},
		{
			name:  "lockfile bump alongside bead churn builds",
			paths: []string{".beads/checkpoint/current.json", "firmware/dependencies.lock"},
			want:  true,
		},
		{
			name:  "manifest change alongside docs builds",
			paths: []string{"docs/notes/changelog.md", "firmware/main/idf_component.yml"},
			want:  true,
		},
		{
			name:  "vendored component edit alongside bead churn builds",
			paths: []string{".beads/checkpoint/objects/abc.jsonl", "firmware/managed_components/espressif__mdns/mdns.c"},
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pushTriggersBuild(tt.paths...); got != tt.want {
				t.Errorf("pushTriggersBuild(%v) = %v, want %v", tt.paths, got, tt.want)
			}
		})
	}
}

// TestCoreTreePathClassesTriggerBuild pins the positive side of the trigger
// contract for the tree classes the filter chiefly exists to protect:
// mothership application code, dashboard runtime assets, and root build
// configuration. The exclusion-based predicate makes these trigger by
// construction — until someone adds their prefix to ignoredByLiveFilter,
// which is exactly the drift these rows catch. No row asserts want=false for
// gating test directories (mothership/test/**, dashboard/tests/**): spec
// Tier B forbids ignoring them, so a test-directory path belongs on the
// trigger side too.
func TestCoreTreePathClassesTriggerBuild(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "mothership application source",
			path: "mothership/internal/signal/ambient.go",
			want: true,
		},
		{
			name: "mothership acceptance test source",
			path: "mothership/test/acceptance/as9_person_count_test.go",
			want: true,
		},
		{
			name: "dashboard service worker",
			path: "dashboard/sw.js",
			want: true,
		},
		{
			name: "dashboard PWA manifest",
			path: "dashboard/manifest.json",
			want: true,
		},
		{
			name: "root version pin",
			path: "VERSION",
			want: true,
		},
		{
			name: "image build recipe",
			path: "Dockerfile",
			want: true,
		},
		{
			name: "workspace module file",
			path: "go.work",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := triggersBuild(tt.path); got != tt.want {
				t.Errorf("triggersBuild(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestManagedComponentCarriersArePushable guards the load-bearing premise of
// the carrier design: the two files that carry managed-component changes into
// pushes must never be gitignored. If one of them were ignored, component
// pins could change silently — no push, no sensor fire, no firmware build,
// and an image that quietly drifts from the manifest.
func TestManagedComponentCarriersArePushable(t *testing.T) {
	const carriers = "../../../.gitignore"

	raw, err := os.ReadFile(carriers)
	if err != nil {
		t.Fatalf("read repo .gitignore: %v", err)
	}

	// The vendored directory must stay ignored — that is exactly why the
	// carrier files are the trigger surface. If this rule disappears, the
	// vendored trees become direct (and enormous) trigger paths; reconcile
	// docs/build-path-filter-spec.md §1.2 and the declarative-config sensor
	// before removing it.
	if !strings.Contains(string(raw), "firmware/managed_components/") {
		t.Error(".gitignore no longer ignores firmware/managed_components/; the managed-component trigger surface must be re-reconciled (spec §1.2)")
	}

	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, carrier := range []string{"firmware/main/idf_component.yml", "firmware/dependencies.lock"} {
			if strings.Contains(trimmed, carrier) {
				t.Errorf(".gitignore pattern %q would ignore managed-component carrier %s — component changes would stop triggering builds", trimmed, carrier)
			}
		}
	}
}

// TestManagedComponentCarrierFilesExist pins that the carrier files are
// present and do carry managed-component declarations, so the trigger
// contract in the tests above describes real files rather than hypothetical
// paths.
func TestManagedComponentCarrierFilesExist(t *testing.T) {
	tests := []struct {
		file    string
		mustSay string
	}{
		{"../../../firmware/main/idf_component.yml", "espressif/"},
		{"../../../firmware/dependencies.lock", "espressif/"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			raw, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatalf("managed-component carrier file missing: %v", err)
			}
			if !strings.Contains(string(raw), tt.mustSay) {
				t.Errorf("%s no longer references a managed component (%q); the carrier-path trigger contract must be re-pointed at wherever component declarations moved", tt.file, tt.mustSay)
			}
		})
	}
}
