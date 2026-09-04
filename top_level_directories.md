# Spaxel Repository - Top-Level Directory Catalog

**Generated:** 2026-08-28
**Refreshed:** 2026-09-03 (adds `memory/` and `tmp/`, corrects hidden/visible counts)
**Total Directories:** 18 (excluding root `.`)
**Purpose:** Comprehensive listing of all top-level directories for discovery and navigation

---

## Directory List

| # | Directory | Purpose |
|---|-----------|---------|
| 1 | `~` | Untracked scratch directory (tilde-named, not ignored, never committed) |
| 2 | `.beads` | Bead tracking system workspace (`.beads/checkpoint/`, `.beads/config.json`) |
| 3 | `.claude` | Claude Code configuration and local settings (untracked) |
| 4 | `.git` | Git repository metadata (standard Git directory) |
| 5 | `.marathon` | Marathon test configuration (test runner settings) |
| 6 | `cmd` | Command-line entry points (Go modules: `spaxel-sim` simulator CLI) |
| 7 | `dashboard` | Web UI assets (HTML, JS, Three.js, CSS - embedded into binary via `go:embed`) |
| 8 | `data` | Persistent data directory mount (SQLite DB, floor plans, CSI replay buffer, firmware uploads) |
| 9 | `docs` | Documentation tree (notes/, research/, plan/plan.md - architecture, ADRs, research) |
| 10 | `firmware` | ESP-IDF ESP32-S3 firmware project (C source, CMakeLists, partitions.csv, sdkconfig) |
| 11 | `memory` | Persistent agent memory notes (e.g. `memory/ssh-access-k3s-agent-minisforum.md`) |
| 12 | `mothership` | Go backend mothership module (`cmd/mothership/`, `internal/` packages - main application) |
| 13 | `notes` | Additional notes directory (supplemental project notes) |
| 14 | `scripts` | Utility scripts (shell scripts for various operations) |
| 15 | `test` | Test directory (likely Go module or acceptance tests) |
| 16 | `testdata` | Test data fixtures and sample files |
| 17 | `tests` | Additional test directory (shell-based e2e test harness: `tests/e2e/run.sh`) |
| 18 | `tmp` | Local scratch directory (gitignored via `tmp/` in `.gitignore`) |

---

## Summary

**Total:** 18 top-level directories (excluding `.`)

**Key Components:**
- **Backend:** `mothership/` (Go), `cmd/` (CLI entry points)
- **Frontend:** `dashboard/` (embedded static assets)
- **Firmware:** `firmware/` (ESP32-S3 ESP-IDF project)
- **Testing:** `test/`, `testdata/`, `tests/` (multiple test suites)
- **Documentation:** `docs/` (architecture, ADRs, plan), `notes/` (supplemental notes)
- **Data:** `data/` (persistent SQLite, floor plans, CSI replay)
- **Configuration:** `.beads/` (bead tracking), `.claude/` (Claude Code config), `.marathon/` (test runner)
- **Agent state:** `memory/` (tracked memory notes), `tmp/` (ignored scratch)

**Hidden Directories:** 4 (`.beads`, `.claude`, `.git`, `.marathon`)
**Visible Directories:** 14 (`~`, `cmd`, `dashboard`, `data`, `docs`, `firmware`, `memory`, `mothership`, `notes`, `scripts`, `test`, `testdata`, `tests`, `tmp`)

**Tracked vs. untracked:** 14 of the 18 are tracked in git at HEAD (`.beads`, `.marathon`, `cmd`, `dashboard`, `data`, `docs`, `firmware`, `memory`, `mothership`, `notes`, `scripts`, `test`, `testdata`, `tests`). The other 4 are working-tree artifacts: `~` and `.claude` are untracked-but-not-ignored, `.git` is Git metadata, and `tmp/` is gitignored.

---

## Discovery Notes

- Repository follows multi-module Go workspace structure (`go.work`)
- Mothership is the primary backend module (Go-based)
- Dashboard is embedded at build time, not served as separate volume
- Firmware is ESP-IDF C project, cross-compiled during Docker build
- Multiple test suites exist: Go unit tests, acceptance tests, and shell-based e2e tests
- Bead system (`.beads/`) tracks work items using bead-rs CLI
- Claude Code configuration (`.claude/`) present for agent/harness configuration
