//go:build embed

package main

import "embed"

//go:embed dashboard
var embeddedDashboardFS embed.FS

func init() {
	dashboardFS = embeddedDashboardFS
	dashboardEmbedded = true
}
