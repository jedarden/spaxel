# Spaxel Repository - Top-Level Directory Catalog

**Generated:** 2026-08-28
**Total Directories:** 16 (excluding root `.`)
**Purpose:** Comprehensive listing of all top-level directories for discovery and navigation

---

## Directory List

| # | Directory | Purpose |
|---|-----------|---------|
| 1 | `~` | Temporary/scratch directory (tilde-prefixed for workspace organization) |
| 2 | `.beads` | Bead tracking system workspace (`.beads/checkpoint/`, `.beads/config.json`) |
| 3 | `.claude` | Claude Code configuration and local settings |
| 4 | `cmd` | Command-line entry points (Go modules: `spaxel-sim` simulator CLI) |
| 5 | `dashboard` | Web UI assets (HTML, JS, Three.js, CSS - embedded into binary via `go:embed`) |
| 6 | `data` | Persistent data directory mount (SQLite DB, floor plans, CSI replay buffer, firmware uploads) |
| 7 | `docs` | Documentation tree (notes/, research/, plan/plan.md - architecture, ADRs, research) |
| 8 | `firmware` | ESP-IDF ESP32-S3 firmware project (C source, CMakeLists, partitions.csv, sdkconfig) |
| 9 | `.git` | Git repository metadata (standard Git directory) |
| 10 | `.marathon` | Marathon test configuration (test runner settings) |
| 11 | `mothership` | Go backend mothership module (`cmd/mothership/`, `internal/` packages - main application) |
| 12 | `notes` | Additional notes directory (supplemental project notes) |
| 13 | `scripts` | Utility scripts (shell scripts for various operations) |
| 14 | `test` | Test directory (likely Go module or acceptance tests) |
| 15 | `testdata` | Test data fixtures and sample files |
| 16 | `tests` | Additional test directory (shell-based e2e test harness: `tests/e2e/run.sh`) |

---

## Summary

**Total:** 16 top-level directories (excluding `.`)

**Key Components:**
- **Backend:** `mothership/` (Go), `cmd/` (CLI entry points)
- **Frontend:** `dashboard/` (embedded static assets)
- **Firmware:** `firmware/` (ESP32-S3 ESP-IDF project)
- **Testing:** `test/`, `testdata/`, `tests/` (multiple test suites)
- **Documentation:** `docs/` (architecture, ADRs, plan), `notes/` (supplemental notes)
- **Data:** `data/` (persistent SQLite, floor plans, CSI replay)
- **Configuration:** `.beads/` (bead tracking), `.claude/` (Claude Code config), `.marathon/` (test runner)

**Hidden Directories:** 6 (`.beads`, `.claude`, `.git`, `.marathon`, plus unlisted dotfiles)
**Visible Directories:** 10 (`~`, `cmd`, `dashboard`, `data`, `docs`, `firmware`, `mothership`, `notes`, `scripts`, `test`, `testdata`, `tests`)

---

## Discovery Notes

- Repository follows multi-module Go workspace structure (`go.work`)
- Mothership is the primary backend module (Go-based)
- Dashboard is embedded at build time, not served as separate volume
- Firmware is ESP-IDF C project, cross-compiled during Docker build
- Multiple test suites exist: Go unit tests, acceptance tests, and shell-based e2e tests
- Bead system (`.beads/`) tracks work items using bead-rs CLI
- Claude Code configuration (`.claude/`) present for agent/harness configuration
