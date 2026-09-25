package auth

// This file pins the mothership's entire HTTP route surface against the
// documented authentication model. Every route the module registers must be
// either explicitly public (auth.IsPublicPath / isStaticAsset) or explicitly
// documented here as PIN-session-gated by auth.Handler.Middleware. A new
// route registered anywhere in the module without a documented auth
// expectation fails these tests with the offending route named.
//
// The specification for the auth model lives in docs/notes/dashboard-pin-auth.md.
//
// How the surface is enumerated: a deterministic, offline source scan of
// route-registration call sites in cmd/mothership/ and internal/ — everything
// that compiles into the shipped mothership binary — in the same spirit as
// internal/privacy's egress allowlist test. There is no in-process seam that
// builds the production router (registrations are spread across run() in
// cmd/mothership/main.go and per-package RegisterRoutes methods over live
// subsystems), so the scan walks the registration call sites directly: chi's
// .Get/.Post/.../.Handle/.HandleFunc with literal paths, including
// .Route("/prefix", ...) nesting and the "METHOD /path" HandleFunc form.
//
// Production wiring note: the in-app PIN middleware is intentionally NOT
// installed on the production router — access control is terminated upstream
// at the Traefik forward-auth layer (Google OAuth); see commit 821b3823.
// What this file pins is the in-app gate itself: the Middleware decision
// contract that protects every non-public route wherever the middleware is
// installed (embedded/offline deployments, defense in depth). This is a
// property of auth.Handler, not a claim that cmd/mothership installs it —
// do not "fix" a failure here by wiring the middleware into main.go.
//
// On failure: if you registered a new route, add it to documentedGatedRoutes
// below (the default and almost always correct classification), or — only if
// it genuinely must be reachable without a session per the documented model —
// to auth.IsPublicPath and documentedPublicRoutes, with a justification.
// Removing a route means removing its documented entry: the inventory
// documents the surface as it is, not as it used to be.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// Route-registration scanner
// ---------------------------------------------------------------------------

// routeRegistration is one route-registration call site found in module source.
type routeRegistration struct {
	method  string // "GET", "POST", ... or "*" for method-agnostic .Handle/.HandleFunc
	pattern string // chi path pattern, Route-prefix stacking already applied
	file    string // module-root-relative, slash-separated
	line    int    // 1-based
}

func (r routeRegistration) key() string { return r.method + " " + r.pattern }

func (r routeRegistration) String() string {
	return fmt.Sprintf("%-6s %-58s %s:%d", r.method, r.pattern, r.file, r.line)
}

// registrationKind distinguishes the three registration shapes the scanner
// matches; each has a different group layout in its regex.
type registrationKind int

const (
	regKindMethod registrationKind = iota // .Get("/path", h): group 1 = method, group 2 = literal
	regKindHandle                         // .Handle("/path", h) / .HandleFunc("METHOD /path", h): group 1 = name, group 2 = literal
	regKindRoute                          // .Route("/prefix", func(...)): group 1 = literal prefix
)

// registrationPatterns match Go constructs that register an HTTP route with a
// literal path. They are matched line-by-line against non-test module sources.
// In every regex the path/prefix literal is the LAST capture group.
var registrationPatterns = []struct {
	re   *regexp.Regexp
	kind registrationKind
}{
	// chi-style method helpers: r.Get("/path", h)
	{regexp.MustCompile(`\.(Get|Post|Put|Patch|Delete|Head|Options)\(\s*("(?:[^"\\]|\\.)*")`), regKindMethod},
	// .Handle("/path", h) / .HandleFunc("/path", h) / .HandleFunc("METHOD /path", h)
	{regexp.MustCompile(`\.(Handle|HandleFunc)\(\s*("(?:[^"\\]|\\.)*")`), regKindHandle},
	// .Route("/prefix", func(r chi.Router) { ... }) — prefix stacking; the
	// scanner tracks the matched braces and joins inner registrations.
	{regexp.MustCompile(`\.Route\(\s*("(?:[^"\\]|\\.)*")\s*,\s*func\(`), regKindRoute},
}

// dynamicRouterReceivers are the receiver identifiers route registrations use
// in this module (r in main.go; router/hfRouter in RegisterRoutes methods).
// The dynamic-path check below fires only for these receivers, so ordinary
// .Get/.Delete calls on stores, generators and clients are not flagged. If
// you register routes through a receiver with a new name, add it here — and
// prefer a literal path so the surface stays enumerable.
const dynamicRouterReceivers = `(?:r|router|hfRouter|mux|sub|group)`

// unregisteredArgument matches a route-registration call whose path argument
// is not a string literal — a dynamically built path. It runs on comment- and
// string-blanked code, where a literal argument has become whitespace
// followed by the argument separator, so the class below excludes quotes,
// whitespace, commas and the closing paren: what remains is a real,
// non-literal first argument. Commented-out registrations never fire it.
var unregisteredArgument = regexp.MustCompile(
	`(?:^|[^.\w])` + dynamicRouterReceivers +
		`\.(?:Get|Post|Put|Patch|Delete|Head|Options|Handle|HandleFunc|Mount|Route)\(\s*[^"\s,)]`)

// registrationSuppressed matches line shapes that textually look like route
// registrations but are not: .Get is also the idiom for reading query
// parameters and header values.
var registrationSuppressed = regexp.MustCompile(`Query\(\)\s*\.Get\(|Header\.Get\(`)

// lexLine walks one source line honoring double-quoted strings, backtick raw
// strings, rune literals and // comments. It returns the line with literal
// contents and comments blanked (safe for brace counting — chi path patterns
// like "/api/{id}" contain braces that must not count), plus the string
// literal contents in order. inRaw carries raw-string state across lines.
func lexLine(line string, inRaw *bool) (code string) {
	var b strings.Builder
	i, n := 0, len(line)
	for i < n {
		c := line[i]
		if *inRaw {
			if c == '`' {
				*inRaw = false
			}
			b.WriteByte(' ')
			i++
			continue
		}
		switch c {
		case '`':
			*inRaw = true
			b.WriteByte(' ')
			i++
		case '"':
			b.WriteByte(' ')
			i++
			for i < n && line[i] != '"' {
				b.WriteByte(' ')
				if line[i] == '\\' {
					i++
					if i < n {
						b.WriteByte(' ')
					}
				}
				i++
			}
			if i < n {
				b.WriteByte(' ')
				i++ // closing quote
			}
		case '\'':
			b.WriteByte(' ')
			i++
			if i < n && line[i] == '\\' {
				b.WriteByte(' ')
				i++
			}
			if i < n {
				b.WriteByte(' ')
				i++
			}
			if i < n && line[i] == '\'' {
				b.WriteByte(' ')
				i++
			}
		case '/':
			if i+1 < n && line[i+1] == '/' {
				i = n // comment: nothing but whitespace remains
				continue
			}
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// literalAt returns the unquoted contents of the string literal starting at
// byte offset start of line, or false if no string literal starts there.
func literalAt(line string, start int) (string, bool) {
	if start >= len(line) || line[start] != '"' {
		return "", false
	}
	var lit strings.Builder
	i := start + 1
	for i < len(line) {
		switch line[i] {
		case '\\':
			if i+1 < len(line) {
				lit.WriteByte(line[i+1])
			}
			i += 2
		case '"':
			return lit.String(), true
		default:
			lit.WriteByte(line[i])
			i++
		}
	}
	return "", false
}

// buildIgnored reports whether a Go source carries a `//go:build ignore`
// constraint in its leading comment block. Such files never compile into the
// shipped binary.
func buildIgnored(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//") {
			return false
		}
		fields := strings.Fields(strings.TrimPrefix(trimmed, "//"))
		if len(fields) == 2 && (fields[0] == "go:build" || fields[0] == "+build") && fields[1] == "ignore" {
			return true
		}
	}
	return false
}

// routeSurfaceModuleRoot locates the mothership module root (the directory
// containing go.mod) relative to this test file.
func routeSurfaceModuleRoot(t *testing.T) string {
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

// scanRouteRegistrations walks cmd/mothership/ and internal/ under root and
// returns every route-registration call site in non-test, non-build-ignored
// .go files, plus the number of files inspected. _test.go sources are skipped:
// they are not part of the binary, and test code legitimately mounts its own
// routers. Results are sorted by file, then line, for deterministic output.
func scanRouteRegistrations(t *testing.T, root string) (regs []routeRegistration, filesScanned int) {
	t.Helper()
	var dynamic []string
	for _, dir := range []string{"cmd/mothership", "internal"} {
		absDir := filepath.Join(root, dir)
		err := filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != absDir && d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			filesScanned++
			if buildIgnored(src) {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)

			var prefixes []string // open .Route(...) prefixes
			var depths []int      // brace depth each prefix opened at
			depth := 0
			inRaw := false
			for i, line := range strings.Split(string(src), "\n") {
				code := lexLine(line, &inRaw)

				if registrationSuppressed.MatchString(line) {
					// Query/header reads, not route registrations. The brace
					// accounting below still runs on this line.
				} else {
					if unregisteredArgument.MatchString(code) {
						dynamic = append(dynamic, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
					}
					for _, rp := range registrationPatterns {
						for _, m := range rp.re.FindAllStringSubmatchIndex(line, -1) {
							lit, ok := literalAt(line, m[len(m)-2])
							if !ok {
								continue
							}
							switch rp.kind {
							case regKindRoute:
								prefixes = append(prefixes, lit)
								depths = append(depths, -1)
							case regKindMethod:
								regs = append(regs, routeRegistration{
									method:  strings.ToUpper(line[m[2]:m[3]]),
									pattern: joinRoutePrefixes(prefixes, lit),
									file:    rel,
									line:    i + 1,
								})
							case regKindHandle:
								// Method may be prefixed in the pattern
								// ("GET /api/auth/status").
								reg := routeRegistration{method: "*", pattern: lit, file: rel, line: i + 1}
								if idx := strings.Index(lit, " "); idx > 0 && isUpperToken(lit[:idx]) {
									reg.method = lit[:idx]
									reg.pattern = lit[idx+1:]
								}
								reg.pattern = joinRoutePrefixes(prefixes, reg.pattern)
								regs = append(regs, reg)
							}
						}
					}
				}

				for _, ch := range code {
					if ch == '{' {
						depth++
					} else if ch == '}' {
						depth--
						if len(depths) > 0 && depths[len(depths)-1] >= 0 && depth < depths[len(depths)-1] {
							depths = depths[:len(depths)-1]
							prefixes = prefixes[:len(prefixes)-1]
						}
					}
				}
				for j, d := range depths {
					if d == -1 {
						depths[j] = depth
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scanning %s: %v", dir, err)
		}
	}
	sort.Slice(regs, func(i, j int) bool {
		if regs[i].file != regs[j].file {
			return regs[i].file < regs[j].file
		}
		return regs[i].line < regs[j].line
	})
	if len(dynamic) > 0 {
		t.Fatalf("found %d route registration(s) whose path is not a string literal; route registration must use literal paths so the surface stays enumerable:\n  %s",
			len(dynamic), strings.Join(dynamic, "\n  "))
	}
	return regs, filesScanned
}

// joinRoutePrefixes joins the open .Route(...) prefixes (innermost last)
// onto a registration's pattern.
func joinRoutePrefixes(prefixes []string, pattern string) string {
	if len(prefixes) == 0 {
		return pattern
	}
	return strings.Join(prefixes, "") + pattern
}

func isUpperToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Documented route inventory
// ---------------------------------------------------------------------------

// documentedPublicRoutes is the complete set of scanned routes that are
// reachable without a session. Key: "METHOD pattern" (* = any method).
// Value: why the route is public under the documented model. The test derives
// the classification by calling the real auth.IsPublicPath, so this map and
// the code cannot drift silently.
//
// Static assets (login-page JS/CSS/images under /js/, /css/, /images/,
// /favicon) are exempted by isStaticAsset rather than IsPublicPath and are
// served through the catch-all registration below rather than their own route
// entries; the behavioral test probes them by concrete path.
var documentedPublicRoutes = map[string]string{
	"GET /healthz":                    "liveness probe for startup self-check and orchestration",
	"GET /api/auth/status":            "login screen must learn PIN-configured/demo state before authenticating",
	"POST /api/auth/setup":            "first-run PIN setup; refused once a PIN exists",
	"POST /api/auth/login":            "login itself must be reachable unauthenticated",
	"POST /api/auth/logout":           "logout must be reachable unauthenticated",
	"POST /api/provision":             "node provisioning handshake; nodes have no session yet",
	"* /ws/node":                      "node CSI ingestion WebSocket; nodes authenticate with per-node HMAC install-secret tokens, not PIN sessions",
	"GET /firmware/{filename}":        "firmware artifact download; URL carries a SHA256 for integrity",
	"GET /firmware/serial/{filename}": "serial-mode firmware artifact download; same model as /firmware/",
}

// documentedGatedRoutes is every other registration in the module: routes
// that must never serve an unauthenticated request. Anything registered but
// absent from this list (and from documentedPublicRoutes) fails the test.
var documentedGatedRoutes = []string{
	"* /metrics",
	"* /ws/dashboard",
	"DELETE /api/automations/volumes/{id}",
	"DELETE /api/automations/{id}",
	"DELETE /api/ble/devices/{mac}",
	"DELETE /api/nodes/{mac}",
	"DELETE /api/notifications/channels/{id}",
	"DELETE /api/people/{id}",
	"DELETE /api/portals/{id}",
	"DELETE /api/simulator/nodes/{id}",
	"DELETE /api/simulator/nodes/{nodeID}/",
	"DELETE /api/simulator/walkers/{id}",
	"DELETE /api/simulator/walkers/{walkerID}",
	"DELETE /api/triggers/{id}",
	"DELETE /api/zones/{id}",
	"GET /*",
	"GET /ambient",
	"GET /api/accuracy/position",
	"GET /api/accuracy/position/history",
	"GET /api/accuracy/samples",
	"GET /api/accuracy/samples/recent",
	"GET /api/accuracy/weights",
	"GET /api/accuracy/weights/{zoneX}/{zoneY}",
	"GET /api/alerts/active",
	"GET /api/analytics/corridors",
	"GET /api/analytics/dwell",
	"GET /api/analytics/flow",
	"GET /api/anomalies",
	"GET /api/anomalies/active",
	"GET /api/anomalies/history",
	"GET /api/anomalies/learning",
	"GET /api/anomalies/summary",
	"GET /api/anomaly_patterns",
	"GET /api/auth/install-secret",
	"GET /api/automations",
	"GET /api/automations/events",
	"GET /api/automations/volumes",
	"GET /api/automations/{id}",
	"GET /api/backup",
	"GET /api/baseline",
	"GET /api/ble/devices",
	"GET /api/ble/devices/{mac}",
	"GET /api/ble/devices/{mac}/aliases",
	"GET /api/ble/devices/{mac}/history",
	"GET /api/ble/duplicates",
	"GET /api/ble/matches",
	"GET /api/blobs",
	"GET /api/briefing",
	"GET /api/briefing/latest",
	"GET /api/briefing/settings",
	"GET /api/briefing/today",
	"GET /api/briefing/{date}",
	"GET /api/coverage",
	"GET /api/coverage/history",
	"GET /api/diagnostics",
	"GET /api/diagnostics/link/{linkID}",
	"GET /api/diskspace/stats",
	"GET /api/diurnal/slots/{linkID}",
	"GET /api/diurnal/status",
	"GET /api/diurnal/status/{linkID}",
	"GET /api/doctor",
	"GET /api/events",
	"GET /api/events/{id}",
	"GET /api/explain/blob/{blobID}/at/{timestamp}",
	"GET /api/explain/{blobID}",
	"GET /api/export",
	"GET /api/firmware",
	"GET /api/firmware/manifest",
	"GET /api/firmware/progress",
	"GET /api/fleet",
	"GET /api/fleet/events/{id}",
	"GET /api/fleet/health",
	"GET /api/fleet/history",
	"GET /api/fleet/simulate",
	"GET /api/floorplan",
	"GET /api/floorplan/calibrate",
	"GET /api/floorplan/image",
	"GET /api/framestats/all",
	"GET /api/framestats/link/{linkID}",
	"GET /api/guided/issues",
	"GET /api/guided/node/{mac}/troubleshoot",
	"GET /api/guided/tooltip/{featureId}",
	"GET /api/healing/status",
	"GET /api/healing/suggest",
	"GET /api/health/system",
	"GET /api/help/notifications",
	"GET /api/learning/accuracy",
	"GET /api/learning/accuracy/history",
	"GET /api/learning/accuracy/improvement",
	"GET /api/learning/feedback",
	"GET /api/learning/feedback/{eventID}",
	"GET /api/learning/stats",
	"GET /api/links",
	"GET /api/links/{linkID}/diagnostics",
	"GET /api/links/{linkID}/health-history",
	"GET /api/localization/accuracy/current",
	"GET /api/localization/accuracy/history",
	"GET /api/localization/accuracy/improvement",
	"GET /api/localization/ground-truth",
	"GET /api/localization/groundtruth/samples",
	"GET /api/localization/groundtruth/stats",
	"GET /api/localization/improvement",
	"GET /api/localization/learning/history",
	"GET /api/localization/learning/progress",
	"GET /api/localization/progress",
	"GET /api/localization/self-improving/status",
	"GET /api/localization/sigmas",
	"GET /api/localization/spatial-weights",
	"GET /api/localization/spatial-weights/stats",
	"GET /api/localization/spatial-weights/zone/{zoneX}/{zoneY}",
	"GET /api/localization/stats",
	"GET /api/localization/weights",
	"GET /api/localization/weights/stats",
	"GET /api/localization/weights/{linkID}",
	"GET /api/mode",
	"GET /api/nodes",
	"GET /api/nodes/{mac}",
	"GET /api/notifications/channels",
	"GET /api/notifications/config",
	"GET /api/notifications/history",
	"GET /api/notifications/preview",
	"GET /api/occupancy",
	"GET /api/ota/auto/config",
	"GET /api/ota/auto/drift",
	"GET /api/ota/auto/history",
	"GET /api/ota/auto/status",
	"GET /api/people",
	"GET /api/people/{id}",
	"GET /api/portals",
	"GET /api/portals/{id}/crossings",
	"GET /api/predictions",
	"GET /api/predictions/accuracy",
	"GET /api/predictions/accuracy/overall",
	"GET /api/predictions/accuracy/{personID}",
	"GET /api/predictions/horizon",
	"GET /api/predictions/horizon/{personID}",
	"GET /api/predictions/patterns/zone/{zoneID}",
	"GET /api/predictions/patterns/zone/{zoneID}/current",
	"GET /api/predictions/patterns/zones",
	"GET /api/predictions/patterns/zones/{zoneID}",
	"GET /api/predictions/pending",
	"GET /api/predictions/probabilities/{personID}",
	"GET /api/predictions/probabilities/{personID}/zone/{zoneID}",
	"GET /api/predictions/probabilities/{personID}/zone/{zoneID}/hour/{hour}",
	"GET /api/predictions/samples/{personID}/zone/{zoneID}",
	"GET /api/predictions/stats",
	"GET /api/replay/session/{id}",
	"GET /api/replay/sessions",
	"GET /api/security",
	"GET /api/security/status",
	"GET /api/settings",
	"GET /api/settings/integration",
	"GET /api/settings/network",
	"GET /api/settings/network/recovery",
	"GET /api/settings/notifications",
	"GET /api/simulator/",
	"GET /api/simulator/gdop/coverage",
	"GET /api/simulator/gdop/heatmap",
	"GET /api/simulator/nodes",
	"GET /api/simulator/nodes/",
	"GET /api/simulator/results",
	"GET /api/simulator/shopping-list",
	"GET /api/simulator/space",
	"GET /api/simulator/space/",
	"GET /api/simulator/status",
	"GET /api/simulator/walkers",
	"GET /api/simulator/walkers/",
	"GET /api/sleep",
	"GET /api/sleep/reports",
	"GET /api/sleep/reports/{linkID}",
	"GET /api/sleep/sessions",
	"GET /api/sleep/sessions/{linkID}",
	"GET /api/sleep/sessions/{linkID}/samples",
	"GET /api/sleep/status",
	"GET /api/sleep/summary",
	"GET /api/status",
	"GET /api/tracks",
	"GET /api/triggers",
	"GET /api/triggers/log",
	"GET /api/triggers/{id}",
	"GET /api/triggers/{id}/webhook-log",
	"GET /api/weather",
	"GET /api/weather/summary",
	"GET /api/weather/{linkID}",
	"GET /api/weather/{linkID}/weekly",
	"GET /api/zones",
	"GET /api/zones/{id}/history",
	"GET /fleet",
	"GET /floorplan/image.png",
	"GET /live",
	"GET /setup",
	"GET /simple",
	"HEAD /*",
	"PATCH /api/briefing/settings",
	"PATCH /api/nodes/{mac}/label",
	"PATCH /api/replay/params",
	"PATCH /api/settings",
	"POST /api/accuracy/position/compute",
	"POST /api/alerts/{id}/acknowledge",
	"POST /api/anomalies/model/update",
	"POST /api/anomalies/{id}/acknowledge",
	"POST /api/auth/change-pin",
	"POST /api/automations",
	"POST /api/automations/volumes",
	"POST /api/automations/{id}/test",
	"POST /api/baseline/capture",
	"POST /api/ble/devices/preregister",
	"POST /api/ble/merge",
	"POST /api/ble/split",
	"POST /api/briefing/generate",
	"POST /api/briefing/test",
	"POST /api/briefing/{id}/acknowledge",
	"POST /api/events/{id}/feedback",
	"POST /api/explain/refresh",
	"POST /api/fall/{id}/acknowledge",
	"POST /api/feedback",
	"POST /api/firmware/ota-all",
	"POST /api/firmware/upload",
	"POST /api/fleet/optimise",
	"POST /api/floorplan/calibrate",
	"POST /api/floorplan/image",
	"POST /api/guided/calibration/complete",
	"POST /api/guided/feedback/response",
	"POST /api/guided/issues/quality/{zoneId}/dismiss",
	"POST /api/guided/tooltip/{featureId}/dismiss",
	"POST /api/help/notifications/test",
	"POST /api/help/notifications/{eventID}/acknowledge",
	"POST /api/import",
	"POST /api/learning/feedback",
	"POST /api/learning/process",
	"POST /api/localization/groundtruth/compute-accuracy",
	"POST /api/localization/reset",
	"POST /api/localization/self-improving/process",
	"POST /api/localization/weights/reset",
	"POST /api/mode",
	"POST /api/nodes/rebaseline-all",
	"POST /api/nodes/update-all",
	"POST /api/nodes/virtual",
	"POST /api/nodes/{mac}/disable",
	"POST /api/nodes/{mac}/enable",
	"POST /api/nodes/{mac}/identify",
	"POST /api/nodes/{mac}/locate",
	"POST /api/nodes/{mac}/ota",
	"POST /api/nodes/{mac}/reboot",
	"POST /api/nodes/{mac}/role",
	"POST /api/notifications/channels",
	"POST /api/notifications/config",
	"POST /api/notifications/quiet-hours",
	"POST /api/notifications/test",
	"POST /api/ota/auto/cancel",
	"POST /api/ota/auto/trigger",
	"POST /api/people",
	"POST /api/portals",
	"POST /api/predictions/patterns/compute",
	"POST /api/predictions/recompute",
	"POST /api/replay/apply-live",
	"POST /api/replay/jump-to-time",
	"POST /api/replay/seek",
	"POST /api/replay/set-speed",
	"POST /api/replay/set-state",
	"POST /api/replay/start",
	"POST /api/replay/stop",
	"POST /api/replay/tune",
	"POST /api/security/acknowledge-all",
	"POST /api/security/arm",
	"POST /api/security/disarm",
	"POST /api/settings",
	"POST /api/settings/integration",
	"POST /api/settings/integration/test",
	"POST /api/simulator/gdop",
	"POST /api/simulator/gdop/compute",
	"POST /api/simulator/nodes",
	"POST /api/simulator/nodes/",
	"POST /api/simulator/nodes/optimize",
	"POST /api/simulator/nodes/suggest",
	"POST /api/simulator/reset",
	"POST /api/simulator/session",
	"POST /api/simulator/simulate",
	"POST /api/simulator/space/validate",
	"POST /api/simulator/subscribe",
	"POST /api/simulator/walkers",
	"POST /api/simulator/walkers/",
	"POST /api/simulator/walkers/path",
	"POST /api/simulator/walkers/random",
	"POST /api/sleep/reports/generate",
	"POST /api/triggers",
	"POST /api/triggers/{id}/disable",
	"POST /api/triggers/{id}/enable",
	"POST /api/triggers/{id}/test",
	"POST /api/zones",
	"PUT /api/automations/{id}",
	"PUT /api/ble/devices/{mac}",
	"PUT /api/nodes/{mac}/position",
	"PUT /api/people/{id}",
	"PUT /api/portals/{id}",
	"PUT /api/room",
	"PUT /api/settings/network",
	"PUT /api/settings/notifications",
	"PUT /api/simulator/nodes/{nodeID}/",
	"PUT /api/simulator/space",
	"PUT /api/simulator/space/",
	"PUT /api/triggers/{id}",
	"PUT /api/zones/{id}",
}

// routeInventoryAnchors are routes the scanner must always find. They catch a
// silently broken scanner (wrong scope, missed pattern) turning the inventory
// test into a vacuous pass.
var routeInventoryAnchors = []string{
	"GET /healthz",
	"* /ws/node",
	"* /ws/dashboard",
	"GET /*",
	"HEAD /*",
	"* /metrics",
	"GET /api/auth/status",
	"POST /api/auth/setup",
	"POST /api/auth/login",
	"POST /api/auth/logout",
	"POST /api/provision",
	"GET /firmware/{filename}",
	"GET /firmware/serial/{filename}",
	"GET /api/doctor",
	"GET /api/blobs",
	"GET /ambient",
	"GET /simple",
	"GET /floorplan/image.png",
}

// isPublicProbes are paths that must NOT be exempt from auth. They fail if
// IsPublicPath is ever widened past the documented model (e.g. a whole
// prefix like /api/ being made public). /api/auth/install-secret is the
// sharpest probe: it is a registered, deliberately gated route.
var isPublicProbes = []string{
	"/api/nodes",
	"/api/doctor",
	"/api/auth/install-secret",
	"/api/auth/change-pin",
	"/metrics",
	"/floorplan/image.png",
	"/ambient",
	"/",
}

// staticAssetProbes pin the isStaticAsset exemption shape: exactly the
// prefixes the login page needs.
var staticAssetProbes = []string{
	"/js/auth.js",
	"/css/panels.css",
	"/images/logo.png",
	"/favicon.ico",
	"/favicon.png",
}

// notStaticAssetProbes are paths that must not ride the static-asset
// exemption. They sit just OUTSIDE the documented prefixes, which is the
// whole of the exemption's contract: isStaticAsset is a deliberate prefix
// check (handler.go), and path-traversal hardening is the static file
// server's job, not this middleware's — traversal-shaped probes under /js/
// are therefore not failures here.
var notStaticAssetProbes = []string{
	"/api/nodes",
	"/jscript/x.js",
	"/imagesques/x.png",
	"/notfavicon.ico",
	"/",
}

// ---------------------------------------------------------------------------
// Static inventory test
// ---------------------------------------------------------------------------

// TestRouteSurfaceMatchesDocumentedInventory enumerates every route
// registration in the module and asserts each is classified exactly as
// documented: public per the real IsPublicPath and listed in
// documentedPublicRoutes, or listed in documentedGatedRoutes. A new
// registration without a documented auth expectation fails here with the
// offending route named.
func TestRouteSurfaceMatchesDocumentedInventory(t *testing.T) {
	root := routeSurfaceModuleRoot(t)
	regs, filesScanned := scanRouteRegistrations(t, root)

	// Sanity guards so a broken scanner can never turn this into a silent
	// pass.
	if filesScanned < 100 {
		t.Fatalf("sanity: only %d non-test .go files scanned under cmd/mothership/ + internal/; expected a module of 100+ — the scan scope is broken", filesScanned)
	}
	if len(regs) < 200 {
		t.Fatalf("sanity: scanner found only %d route registrations; the patterns or scope are broken", len(regs))
	}
	// The inventory is the SET of method+pattern pairs. The same pair may be
	// registered from more than one call site (parallel chi and HandleFunc
	// RegisterRoutes variants register identical paths); a duplicate cannot
	// widen the auth boundary, so call sites are collapsed silently.
	present := make(map[string]bool, len(regs))
	for _, reg := range regs {
		present[reg.key()] = true
	}
	for _, anchor := range routeInventoryAnchors {
		if !present[anchor] {
			t.Errorf("sanity: anchored route %q not found by the scanner — registration patterns or scan scope are broken", anchor)
		}
	}

	var publicUndocumented, gatedUndocumented []string
	for _, reg := range regs {
		switch {
		case IsPublicPath(reg.pattern):
			if _, ok := documentedPublicRoutes[reg.key()]; !ok {
				publicUndocumented = append(publicUndocumented, reg.String())
			}
		default:
			if !containsString(documentedGatedRoutes, reg.key()) {
				gatedUndocumented = append(gatedUndocumented, reg.String())
			}
		}
	}

	var stalePublic, notReallyPublic, staleGated []string
	for key, why := range documentedPublicRoutes {
		if !present[key] {
			stalePublic = append(stalePublic, fmt.Sprintf("%s (%s)", key, why))
			continue
		}
		if !IsPublicPath(strings.SplitN(key, " ", 2)[1]) {
			notReallyPublic = append(notReallyPublic, key)
		}
	}
	for _, key := range documentedGatedRoutes {
		if !present[key] {
			staleGated = append(staleGated, key)
		}
	}

	for _, p := range isPublicProbes {
		if IsPublicPath(p) {
			t.Errorf("IsPublicPath(%q) = true; this path is documented as gated — the public-path exemption has been widened past the model in docs/notes/dashboard-pin-auth.md", p)
		}
	}
	for _, p := range staticAssetProbes {
		if !isStaticAsset(p) {
			t.Errorf("isStaticAsset(%q) = false; this prefix is documented as a login-page asset exemption", p)
		}
	}
	for _, p := range notStaticAssetProbes {
		if isStaticAsset(p) {
			t.Errorf("isStaticAsset(%q) = true; the static-asset exemption has been widened past the documented prefixes", p)
		}
	}

	report := func(label string, items []string, guidance string) {
		if len(items) == 0 {
			return
		}
		t.Errorf("%s:\n  %s\n\n%s", label, strings.Join(items, "\n  "), guidance)
	}
	report("public routes missing from documentedPublicRoutes (a route is reachable without a session but has no documented justification)", publicUndocumented,
		"Every public route needs an entry in documentedPublicRoutes with a justification — and must be covered by IsPublicPath, or it would not actually be gated-by-exemption. If the route should NOT be public, this is an auth hole: gate it.")
	report("gated routes missing from documentedGatedRoutes (a new route registered without a documented auth expectation)", gatedUndocumented,
		"Add the route to documentedGatedRoutes with the auth expectation recorded — or, only if it genuinely must serve unauthenticated requests per docs/notes/dashboard-pin-auth.md, add it to IsPublicPath and documentedPublicRoutes with a justification.")
	report("documentedPublicRoutes entries for routes that no longer exist (stale documentation)", stalePublic,
		"Remove the entry: the inventory documents the surface as it is, not as it used to be.")
	report("documentedPublicRoutes entries whose pattern IsPublicPath now rejects", notReallyPublic,
		"Reconcile the entry with IsPublicPath: the classification must come from the real function, not a copy.")
	report("documentedGatedRoutes entries for routes that no longer exist (stale documentation)", staleGated,
		"Remove the entries for removed routes.")
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Behavioral test: the middleware gate over the live surface
// ---------------------------------------------------------------------------

// routeReachedHeader and routeReachedBody are set/written by every stub
// handler the behavioral test mounts. Presence of the header proves the
// request passed the auth middleware and reached the route's handler — the
// header, unlike the body, also survives on HEAD responses, which never
// carry a body.
const routeReachedHeader = "X-Route-Probe-Reached"
const routeReachedBody = "ROUTE-REACHED"

// chiParam matches a chi URL parameter segment like {nodeID}.
var chiParam = regexp.MustCompile(`\{[^/{}]+\}`)

// instantiatePattern turns a registered chi pattern into a concrete request
// path that routes to it. The catch-all "/*" becomes "/" (the dashboard root,
// which is exactly how the catch-all is reached in production).
func instantiatePattern(pattern string) string {
	if pattern == "/*" {
		return "/"
	}
	return chiParam.ReplaceAllString(pattern, "probe")
}

// routeSurfaceListener opens the server's listener, honoring the repo-wide
// SPAXEL_E2E_BIND_ADDR override (used by the shared e2e harness to pin a
// port) and falling back to an ephemeral loopback port.
func routeSurfaceListener(t *testing.T) net.Listener {
	t.Helper()
	if addr := os.Getenv("SPAXEL_E2E_BIND_ADDR"); addr != "" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("SPAXEL_E2E_BIND_ADDR=%s: %v", addr, err)
		}
		return ln
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ephemeral listen: %v", err)
	}
	return ln
}

// TestUnauthenticatedAccessOverLiveRouteSurface drives a real HTTP server on
// an ephemeral port whose router carries every route the module registers,
// behind the real auth.Handler.Middleware, and asserts the gate for each:
//
//   - a documented-public route serves its handler without a session;
//   - a gated /api/ or /ws/ route rejects an unauthenticated request with the
//     JSON 401 contract and never reaches the handler;
//   - any other gated route (pages, catch-all) gets the login-only page;
//   - with a valid PIN session, every route serves its handler — proving the
//     gate is the session boundary, not a mis-wired route.
func TestUnauthenticatedAccessOverLiveRouteSurface(t *testing.T) {
	root := routeSurfaceModuleRoot(t)
	regs, _ := scanRouteRegistrations(t, root)

	_, h := newAuthEnv(t)
	cookie := configurePin(t, h)

	// Mount the enumerated surface behind the real middleware. chi tolerates
	// same-position parameters with different names ({date} vs {id}), which
	// the production router also relies on (e.g. /api/briefing/*).
	mux := chi.NewRouter()
	mux.Use(h.Middleware)
	seen := make(map[string]bool, len(regs))
	for _, reg := range regs {
		if seen[reg.key()] {
			continue
		}
		seen[reg.key()] = true
		pattern := reg.pattern
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(routeReachedHeader, "1")
			_, _ = io.WriteString(w, routeReachedBody) //nolint:errcheck // test stub
		})
		if reg.method == "*" {
			mux.Handle(pattern, handler)
		} else {
			mux.Method(reg.method, pattern, handler)
		}
	}

	ln := routeSurfaceListener(t)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	baseURL := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 10 * time.Second}

	probe := func(method, path string, sess *http.Cookie) (*http.Response, string) {
		req, err := http.NewRequest(method, baseURL+path, nil)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		if sess != nil {
			req.AddCookie(sess)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%s %s: read body: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return resp, string(body)
	}

	var failures []string
	fail := func(format string, args ...any) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}

	for _, reg := range regs {
		path := instantiatePattern(reg.pattern)
		method := reg.method
		if method == "*" {
			method = http.MethodGet
		}
		isHEAD := method == http.MethodHead
		reached := func(resp *http.Response) bool {
			return resp.Header.Get(routeReachedHeader) == "1"
		}

		resp, body := probe(method, path, nil)
		if IsPublicPath(reg.pattern) {
			if !reached(resp) {
				fail("%s: public route did not serve its handler unauthenticated: status %d body %.60q",
					reg, resp.StatusCode, body)
			}
		} else {
			if reached(resp) {
				fail("%s: AUTH HOLE — unauthenticated request was served by the handler", reg)
				continue
			}
			isAPI := strings.HasPrefix(reg.pattern, "/api/") || strings.HasPrefix(reg.pattern, "/ws/")
			if isAPI {
				if resp.StatusCode != http.StatusUnauthorized {
					fail("%s: gated API route: unauthenticated status = %d, want 401 (body %.60q)", reg, resp.StatusCode, body)
				} else if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
					fail("%s: gated API route: 401 Content-Type = %q, want application/json", reg, ct)
				}
			} else {
				if resp.StatusCode != http.StatusOK {
					fail("%s: gated page route: unauthenticated status = %d, want 200 (login page)", reg, resp.StatusCode)
				}
				if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
					fail("%s: gated page route: unauthenticated Content-Type = %q, want text/html (login page)", reg, ct)
				}
				// HEAD responses never carry a body, so the login-page
				// marker is only checkable on non-HEAD requests.
				if !isHEAD && !strings.Contains(body, "auth.js") {
					fail("%s: gated page route: unauthenticated response is not the login-only page: %.60q", reg, body)
				}
			}
		}

		resp, _ = probe(method, path, cookie)
		if !reached(resp) {
			fail("%s: request with a valid PIN session did not reach the handler (status %d) — the gate is blocking authenticated use", reg, resp.StatusCode)
		}
	}

	// Static assets ride the isStaticAsset exemption and are served by the
	// catch-all registration.
	for _, p := range staticAssetProbes {
		resp, body := probe(http.MethodGet, p, nil)
		if body != routeReachedBody {
			fail("static asset %s did not serve unauthenticated: status %d body %.60q", p, resp.StatusCode, body)
		}
	}
	for _, p := range notStaticAssetProbes {
		_, body := probe(http.MethodGet, p, nil)
		if body == routeReachedBody {
			fail("not-static path %s was served unauthenticated — the static-asset exemption leaked", p)
		}
	}

	// The dashboard root through the catch-all: login page without a
	// session, handler with one.
	resp, body := probe(http.MethodGet, "/", nil)
	if body == routeReachedBody || !strings.Contains(body, "auth.js") {
		fail("GET /: unauthenticated response is not the login-only page: status %d body %.60q", resp.StatusCode, body)
	}
	resp, body = probe(http.MethodGet, "/", cookie)
	if body != routeReachedBody {
		fail("GET /: request with a valid session did not reach the handler: status %d body %.60q", resp.StatusCode, body)
	}

	if len(failures) > 0 {
		const max = 20
		shown := failures
		more := ""
		if len(shown) > max {
			shown = shown[:max]
			more = fmt.Sprintf("\n  … and %d more", len(failures)-max)
		}
		t.Errorf("%d route-surface auth failure(s):\n  %s%s",
			len(failures), strings.Join(shown, "\n  "), more)
	}
}
