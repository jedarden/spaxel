# dashboard/_dev/ — dev-only harnesses

Files here are development-only verification pages. They are **not** dashboard
entry points and **not** part of the shipped product:

- `go:embed` (the `-tags=embed` production build, `mothership/cmd/mothership/
  dashboard_embed.go`) silently excludes any path segment beginning with `_`,
  so nothing in this directory is compiled into the production image.
- The untagged local build serves the repo-root `dashboard/` tree from disk
  (`registerDashboardStatic` in `mothership/cmd/mothership/main.go`), so the
  harnesses remain reachable in dev, e.g. `/_dev/test-transformcontrols.html`.
- The a11y entry-point coverage guard (`tests/a11y-entrypoint-coverage.spec.js`)
  enumerates top-level `dashboard/*.html` only, so harnesses here are outside
  the WCAG gate by construction.

Add a page here only when it verifies a browser API or library behaviour and
has no user-facing role. Anything a home user is meant to open belongs at the
top level, wired into `tests/accessibility/pages.js` and the a11y gate.
