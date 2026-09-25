//go:build embed

package main

// Packaged-dashboard embed smoke test (spaxel-c4d23bf6).
//
// dashboard_embed.go compiles the repo-root dashboard/ into the production
// binary: the Dockerfile stages it at cmd/mothership/dashboard/ immediately
// before the tagged build, and the //go:embed directive there is the embed
// seam. dashboard_static_test.go exercises only the development filesystem
// fallback (registerDashboardStatic) — until now nothing verified the
// embedded serving path itself. This file only compiles under -tags=embed,
// i.e. as part of the same tagged build that produces the artifact, and
// verifies that all nine HTML entry points and every JS/CSS/asset they
// reference are served with correct content and MIME types.
//
// The staging directory is gitignored (.gitignore: /mothership/cmd/mothership/
// dashboard/) exactly like the Docker build's staging. TestMain refreshes it
// from the canonical repo-root dashboard/ before the tests run, so a stale
// staging copy cannot mask a dashboard change in the serving assertions, and
// TestDashboardEmbedStagingInSync proves the compiled-in tree matches the
// canonical sources byte for byte in both directions.
//
// Run:  cd mothership && go test -tags=embed ./cmd/mothership
//
// On a fresh checkout the tagged build fails to compile before TestMain can
// stage anything ("pattern dashboard: no matching files found") — stage once,
// exactly as the Dockerfile does:
//
//	rsync -a --exclude node_modules dashboard/ mothership/cmd/mothership/dashboard/
//
// after which TestMain keeps the staging refreshed on every run.

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// documentedHTMLEntryPoints is the nine HTML entry points of the dashboard,
// enumerated in docs/notes/dashboard-mothership-integration.md ("HTML entry
// points"). Exact set equality with the embedded tree is the same tripwire
// pattern as the AS-enumeration guard: adding or removing a page must update
// this test deliberately rather than silently changing what ships.
var documentedHTMLEntryPoints = []string{
	"ambient.html",
	"fleet.html",
	"index.html",
	"integrations.html",
	"live.html",
	"setup.html",
	"simple.html",
	"simulator.html",
	"test-transformcontrols.html",
}

// htmlEntryRoutes maps each entry point to the route that serves it. Five are
// named page routes handled by serveEmbeddedFile (main.go); the rest —
// including / for index.html — fall out of the embedded catch-all file server.
var htmlEntryRoutes = map[string]string{
	"index.html":                  "/",
	"ambient.html":                "/ambient",
	"fleet.html":                  "/fleet",
	"live.html":                   "/live",
	"setup.html":                  "/setup",
	"simple.html":                 "/simple",
	"integrations.html":           "/integrations.html",
	"simulator.html":              "/simulator.html",
	"test-transformcontrols.html": "/test-transformcontrols.html",
}

// namedPageRoutes marks the five routes registered through serveEmbeddedFile;
// every other route in htmlEntryRoutes is served by the catch-all file server.
var namedPageRoutes = map[string]bool{
	"/ambient": true,
	"/fleet":   true,
	"/live":    true,
	"/setup":   true,
	"/simple":  true,
}

// repoDashboardRoot is the canonical repo-root dashboard/ tree the staging is
// refreshed from ("" if it cannot be located).
var repoDashboardRoot string

// embedStagingExcluded names repo-root dashboard/ entries never staged into
// the embed copy: node_modules (93 MB of dev-only packages) and the
// Playwright/Jest artifact and spec directories. The production Docker build
// stages the whole tree, but every assertion here covers what the nine entry
// points actually reference — HTML, css/, js/, static/ — none of which lives
// under an excluded path. TestDashboardEmbedStagingInSync applies the same
// predicate to both directions of its comparison.
var embedStagingExcluded = map[string]bool{
	"node_modules":      true,
	"tests":             true,
	"test-results":      true,
	"playwright-report": true,
	".auth":             true,
	"coverage":          true,
}

// findCanonicalDashboardForEmbedTest locates the repo-root dashboard/ sources,
// skipping skip (the gitignored staging copy at <package dir>/dashboard,
// which findDashboardTreeNear would otherwise hit first and make every
// comparison in TestDashboardEmbedStagingInSync circular).
func findCanonicalDashboardForEmbedTest(start, marker, skip string) string {
	dir := start
	for i := 0; i < 12; i++ {
		cand := filepath.Join(dir, "dashboard")
		if cand != skip {
			if _, err := os.Stat(filepath.Join(cand, marker)); err == nil {
				return cand
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached filesystem root
		}
		dir = parent
	}
	return ""
}

// TestMain refreshes the gitignored staging directory from the canonical
// repo-root dashboard/ before the tests run, so the serving assertions below
// always exercise the current dashboard sources, never a stale staging copy.
func TestMain(m *testing.M) {
	if cwd, err := os.Getwd(); err == nil {
		staging := filepath.Join(cwd, "dashboard")
		repoDashboardRoot = findCanonicalDashboardForEmbedTest(cwd, dashboardAssetsMarker, staging)
		if repoDashboardRoot != "" {
			if err := stageDashboardForEmbedTest(repoDashboardRoot, staging); err != nil {
				fmt.Fprintf(os.Stderr, "dashboard embed staging refresh failed: %v\n", err)
				os.Exit(1)
			}
		}
	}
	os.Exit(m.Run())
}

// stageDashboardForEmbedTest replaces dst with a copy of src, pruning the
// embedStagingExcluded directories. The copy is built in a sibling temp dir
// and swapped in only once complete: a half-finished RemoveAll would leave
// the package unable to compile at all ("pattern dashboard: no matching
// files found"), which is worse than a stale staging copy. File-at-a-time
// copy (rather than an external rsync) keeps the refresh working wherever
// `go test` runs.
func stageDashboardForEmbedTest(src, dst string) error {
	tmp := dst + ".embed-staging-tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("clearing previous staging temp: %w", err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("creating staging temp: %w", err)
	}
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		segments := strings.Split(filepath.ToSlash(rel), "/")
		if embedStagingExcluded[segments[0]] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(tmp, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("removing stale staging: %w", err)
	}
	return os.Rename(tmp, dst)
}

// embedMatchesGoEmbedRule reports whether rel is a path go:embed would
// include: go:embed silently excludes any path segment beginning with "." or
// "_", so the staging-sync comparison must exclude them on the repo side too.
func embedMatchesGoEmbedRule(rel string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(segment, ".") || strings.HasPrefix(segment, "_") {
			return false
		}
	}
	return true
}

// TestDashboardEmbedEntryPointsComplete asserts the embedded tree contains
// exactly the nine documented HTML entry points. Losing a page (or gaining
// one) must surface here, at the packaged artifact level, rather than as a
// browser 404.
func TestDashboardEmbedEntryPointsComplete(t *testing.T) {
	matches, err := fs.Glob(dashboardFS, "dashboard/*.html")
	if err != nil {
		t.Fatalf("glob embedded dashboard: %v", err)
	}
	got := make([]string, 0, len(matches))
	for _, m := range matches {
		got = append(got, strings.TrimPrefix(m, "dashboard/"))
	}
	sort.Strings(got)

	want := slices.Clone(documentedHTMLEntryPoints)
	sort.Strings(want)

	if !slices.Equal(got, want) {
		t.Fatalf("embedded HTML entry points = %v, want the documented nine %v (docs/notes/dashboard-mothership-integration.md)", got, want)
	}
	for _, name := range want {
		if _, ok := htmlEntryRoutes[name]; !ok {
			t.Fatalf("entry point %s is embedded but has no route in htmlEntryRoutes", name)
		}
	}
}

// TestDashboardEmbedStagingInSync proves the tree compiled into the binary is
// the canonical repo-root dashboard/, in both directions: every repo file the
// embed would include is present and byte-identical in the embedded FS (a
// stale staging copy), and every embedded file traces back to a repo file
// (leftovers from a removed source).
func TestDashboardEmbedStagingInSync(t *testing.T) {
	if repoDashboardRoot == "" {
		t.Skip("canonical dashboard/ sources not found from test CWD; cannot verify staging sync")
	}
	sub, err := fs.Sub(dashboardFS, "dashboard")
	if err != nil {
		t.Fatalf("fs.Sub(dashboard): %v", err)
	}

	repoFiles := 0
	err = filepath.WalkDir(repoDashboardRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repoDashboardRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if embedStagingExcluded[strings.Split(filepath.ToSlash(rel), "/")[0]] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !embedMatchesGoEmbedRule(rel) {
			return nil
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, readErr := fs.ReadFile(sub, filepath.ToSlash(rel))
		if readErr != nil {
			t.Fatalf("repo file %s is missing from the embedded dashboard (stale staging? TestMain refreshes it — check embedStagingExcluded)", rel)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("embedded %s differs from the repo source (stale staging copy compiled into the binary)", rel)
		}
		repoFiles++
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo dashboard: %v", err)
	}

	embeddedFiles := 0
	err = fs.WalkDir(sub, ".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		want, err := os.ReadFile(filepath.Join(repoDashboardRoot, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("embedded file %s has no counterpart in the repo dashboard (leftover staging content)", path)
		}
		got, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("embedded %s differs from the repo source", path)
		}
		embeddedFiles++
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded dashboard: %v", err)
	}

	if repoFiles == 0 || embeddedFiles == 0 {
		t.Fatalf("staging sync compared nothing (repo=%d, embedded=%d files); the comparison itself is broken", repoFiles, embeddedFiles)
	}
}

// TestDashboardEmbedEntryPointsServed is the heart of the smoke test: every
// HTML entry point, at its production route, returns 200 with text/html and a
// body byte-identical to the compiled-in file. Named page routes are exercised
// through serveEmbeddedFile (the exact handler function main registers); the
// rest through embeddedDashboardFileServer (the exact construction main
// registers for /*) — the same shared-seam reasoning as
// TestDashboardStaticAssets.
func TestDashboardEmbedEntryPointsServed(t *testing.T) {
	fileServer, err := embeddedDashboardFileServer()
	if err != nil {
		t.Fatalf("embeddedDashboardFileServer: %v", err)
	}

	for _, name := range documentedHTMLEntryPoints {
		route := htmlEntryRoutes[name]
		t.Run(route, func(t *testing.T) {
			want, err := fs.ReadFile(dashboardFS, "dashboard/"+name)
			if err != nil {
				t.Fatalf("embedded %s unreadable: %v", name, err)
			}

			var rec *httptest.ResponseRecorder
			if namedPageRoutes[route] {
				rec = httptest.NewRecorder()
				serveEmbeddedFile(rec, httptest.NewRequest(http.MethodGet, route, nil), name)
			} else {
				rec = httptest.NewRecorder()
				fileServer(rec, httptest.NewRequest(http.MethodGet, route, nil))
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want 200 (body=%q)", route, rec.Code, truncateBody(rec.Body.String()))
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("GET %s: Content-Type = %q, want text/html prefix", route, ct)
			}
			if !bytes.Equal(rec.Body.Bytes(), want) {
				t.Fatalf("GET %s: body is %d bytes but embedded %s is %d bytes — served content differs from the compiled-in file", route, rec.Body.Len(), name, len(want))
			}

			// HEAD is registered only for the catch-all (bf-1cgqe): the named
			// page routes are chi GET routes and deliberately 405 on HEAD.
			if namedPageRoutes[route] {
				return
			}
			hrec := httptest.NewRecorder()
			fileServer(hrec, httptest.NewRequest(http.MethodHead, route, nil))
			if hrec.Code != http.StatusOK {
				t.Fatalf("HEAD %s: status = %d, want 200", route, hrec.Code)
			}
			if hct := hrec.Header().Get("Content-Type"); !strings.HasPrefix(hct, "text/html") {
				t.Fatalf("HEAD %s: Content-Type = %q, want text/html prefix", route, hct)
			}
			if hrec.Body.Len() != 0 {
				t.Fatalf("HEAD %s: body must be empty, got %d bytes", route, hrec.Body.Len())
			}
		})
	}
}

// htmlAssetRefRe matches href="/src=" attribute values in the entry-point
// HTML. The pages are hand-written with double-quoted attributes; a full HTML
// parser is unnecessary for a smoke test.
var htmlAssetRefRe = regexp.MustCompile(`(?i)\b(?:href|src)\s*=\s*"([^"]*)"`)

// assetMIME maps an asset extension to the acceptable Content-Type prefixes.
// Unlike the server (mime.TypeByExtension) this table is an independent
// expectation, so a server-side MIME regression cannot hide behind itself;
// text/javascript also accepts application/javascript because OS MIME
// registries disagree, mirroring mimeMatches in dashboard_static_test.go.
var assetMIME = map[string][]string{
	".css":  {"text/css"},
	".js":   {"text/javascript", "application/javascript"},
	".json": {"application/json"},
	".png":  {"image/png"},
	".svg":  {"image/svg+xml"},
	".ico":  {"image/vnd.microsoft.icon", "image/x-icon"},
	".webmanifest": {"application/manifest+json", "application/json"},
	".html": {"text/html"},
}

// localAssetPathForEmbedTest resolves an href/src value from a root-level
// entry point to the absolute path the catch-all file server serves it at,
// reporting false for anything not served from the embedded dashboard
// (external URLs, data: URIs, in-page fragments, query-only links). All nine
// entry points are served at root level, so a relative reference resolves to
// "/<ref>".
func localAssetPathForEmbedTest(ref string) (string, bool) {
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" || strings.HasPrefix(ref, "//") {
		return "", false
	}
	if strings.Contains(ref, ":") {
		return "", false // any scheme (https:, data:, mailto:) — plain paths never contain ':'
	}
	if strings.HasPrefix(ref, "/") {
		return canonicalizeIndexForEmbedTest(ref), true
	}
	return canonicalizeIndexForEmbedTest("/" + ref), true
}

// canonicalizeIndexForEmbedTest maps any ".../index.html" path to its
// directory URL: http.FileServer answers ".../index.html" with a 301 redirect
// to the directory ("//index.html" → "/", and "/index.html" → "/"), and the
// directory route is what the entry-points test already asserts at 200.
func canonicalizeIndexForEmbedTest(path string) string {
	if strings.HasSuffix(path, "index.html") {
		return strings.TrimSuffix(path, "index.html")
	}
	return path
}

// TestDashboardEmbedAssetsServed walks every JS/CSS/icon/manifest reference in
// the nine entry points and verifies the embedded artifact serves each one
// with the correct MIME type and byte-identical content — the packaged
// equivalent of what the a11y specs check against the standalone dashboard
// tree. A referenced asset missing from the embedded tree is a 404 in
// production; a wrong Content-Type breaks module scripts and stylesheets.
func TestDashboardEmbedAssetsServed(t *testing.T) {
	fileServer, err := embeddedDashboardFileServer()
	if err != nil {
		t.Fatalf("embeddedDashboardFileServer: %v", err)
	}

	refs := map[string][]string{} // served path -> referencing entry points
	for _, name := range documentedHTMLEntryPoints {
		html, err := fs.ReadFile(dashboardFS, "dashboard/"+name)
		if err != nil {
			t.Fatalf("embedded %s unreadable: %v", name, err)
		}
		for _, match := range htmlAssetRefRe.FindAllStringSubmatch(string(html), -1) {
			path, ok := localAssetPathForEmbedTest(match[1])
			if !ok {
				continue
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == "" {
				continue // page navigation ("", "/", "/live"), not an asset
			}
			if _, known := assetMIME[ext]; !known {
				t.Fatalf("%s references %q (served at %s): extension not in the smoke test's MIME table — add it to assetMIME", name, match[1], path)
			}
			if !slices.Contains(refs[path], name) {
				refs[path] = append(refs[path], name)
			}
		}
	}
	// Guard against the reference parser silently matching nothing: the nine
	// pages reference ~80 distinct local assets today.
	if len(refs) < 50 {
		t.Fatalf("parsed only %d local asset references across the %d entry points; the href/src parser is matching nothing", len(refs), len(documentedHTMLEntryPoints))
	}

	for _, path := range slices.Sorted(maps.Keys(refs)) {
		t.Run(path, func(t *testing.T) {
			want, err := fs.ReadFile(dashboardFS, "dashboard/"+strings.TrimPrefix(path, "/"))
			if err != nil {
				t.Fatalf("asset %s referenced by %v is not in the embedded dashboard: %v (production would 404)", path, refs[path], err)
			}
			rec := httptest.NewRecorder()
			fileServer(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s (referenced by %v): status = %d, want 200 (body=%q)", path, refs[path], rec.Code, truncateBody(rec.Body.String()))
			}
			ct := rec.Header().Get("Content-Type")
			ext := strings.ToLower(filepath.Ext(path))
			if !slices.ContainsFunc(assetMIME[ext], func(prefix string) bool { return strings.HasPrefix(ct, prefix) }) {
				t.Fatalf("GET %s: Content-Type = %q, want one of %v (the bf-1cgqe regression served text/plain here)", path, ct, assetMIME[ext])
			}
			if !bytes.Equal(rec.Body.Bytes(), want) {
				t.Fatalf("GET %s: body is %d bytes but the embedded asset is %d bytes — served content differs", path, rec.Body.Len(), len(want))
			}
		})
	}
}
