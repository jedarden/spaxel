//go:build embed

package main

import "embed"

// Dockerfile copies the canonical repository-root dashboard/ into this
// package directory immediately before the tagged production build. The
// staging directory is intentionally not tracked; untagged local builds use
// the filesystem fallback in main.go and therefore serve the canonical tree.
//go:embed dashboard
var embeddedDashboardFS embed.FS

func init() {
	dashboardFS = embeddedDashboardFS
	dashboardEmbedded = true
}
