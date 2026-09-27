# CI Accessibility Testing Guide

This document describes the accessibility (a11y) testing integration with Argo Workflows CI.

## Overview

Accessibility tests enforce WCAG 2.1 AA compliance as a CI quality gate for the Spaxel dashboard. These tests run automated checks using axe-core and Playwright, ensuring the dashboard remains accessible to users with disabilities.

**Test location:** `dashboard/tests/a11y*.spec.js` + `dashboard/tests/agentation-mount.spec.js`

**Test runner:** Playwright + @axe-core/playwright

**What it tests:** WCAG 2.1 AA compliance across **all eight** production dashboard
HTML entry points (the set enumerated in `docs/codebase-structure-and-test-patterns.md`
and `docs/repo-structure.md` §8):

| # | Entry point | Covered by |
|---|-------------|------------|
| 1 | `index.html` | `tests/a11y.spec.js` |
| 2 | `live.html` | `tests/a11y.spec.js` + `tests/a11y-dashboard.spec.js` |
| 3 | `fleet.html` | `tests/a11y.spec.js` |
| 4 | `setup.html` | `tests/a11y.spec.js` |
| 5 | `integrations.html` | `tests/a11y.spec.js` |
| 6 | `ambient.html` | `tests/a11y-dashboard.spec.js` |
| 7 | `simple.html` | `tests/a11y-dashboard.spec.js` |
| 8 | `simulator.html` | `tests/a11y-dashboard.spec.js` |

Plus the onboarding flow (wizard steps, `tests/a11y-onboarding.spec.js`) and a
coverage guard (`tests/a11y-entrypoint-coverage.spec.js`) that fails the gate
whenever a top-level `.html` entry point exists in `dashboard/` without a page in
one of the two page specs.

Independently of axe, `tests/agentation-mount.spec.js` asserts on every entry
point that the Agentation feedback toolbar actually mounts (workspace rule:
every UI page mounts Agentation) — the import map resolves `react/jsx-runtime`
to the self-hosted vendor chunk, `/agentation.js` and the chunk serve with a
JavaScript Content-Type, `#agentation-root` attaches, and the toolbar renders
inside `agentation-toolbar`'s shadowRoot (`[data-agentation-root]` — light-DOM
querySelector cannot cross the shadow boundary). It carries its own coverage
guard that fails when a new top-level `dashboard/*.html` entry point ships
without Agentation wiring. A `<script>` tag alone is not evidence: without the
import map the module dies on unresolvable bare specifiers while the page keeps
rendering. Dev-only harnesses live under `dashboard/_dev/`
(go:embed excludes `_`-prefixed path segments, so they never ship in the
production image — see `dashboard/_dev/README.md`) and are outside the gate by
construction; the guard only enumerates top-level `dashboard/*.html`.

**Accessibility standard:** WCAG 2.1 AA (via axe-core tags: `wcag2a`, `wcag2aa`)

## Running Locally

```bash
# Run accessibility tests
cd dashboard
npm ci
npx playwright install chromium
npm run test:a11y

# Run specific accessibility test file
npx playwright test a11y.spec.js

# Run with headed browser (see what's being tested)
npx playwright test a11y.spec.js --headed
```

> On NixOS hosts the `npx playwright install chromium` step does not work — see
> [Running locally on NixOS](#running-locally-on-nixos) for the working recipe.

### Running locally on NixOS

`npx playwright install chromium` cannot work on NixOS, and the failure is not a
corrupt cache. Playwright's downloaded browsers (headless shell and full
chromium alike, under `~/.cache/ms-playwright/`) are built for glibc/FHS
distributions and dynamically linked against system libraries they expect at
`/usr/lib` / `/lib64`. NixOS has neither path, so every Playwright-managed
browser dies at launch with exit code 127
(`libglib-2.0.so.0: cannot open shared object file`). `npx playwright
install-deps` is equally a dead end — it needs root on an FHS system. Deleting
and re-downloading the browser changes nothing; the mismatch is between the
browser build and the OS layout.

The one browser that launches is the **Nix-wrapped chromium** in `/nix/store/`:
its Nix RPATH carries the entire library closure. The store hash changes on
every chromium upgrade, so resolve the path fresh and never copy an old one:

```bash
ls -d /nix/store/*chromium-*/bin/chromium
```

The checked-in [`dashboard/playwright.nix.config.js`](../dashboard/playwright.nix.config.js)
wraps the default config and points `use.launchOptions.executablePath` at that
browser — the nesting matters, because Playwright silently ignores a top-level
`use.executablePath`. The path is resolved at config load (override with the
`SPAXEL_CHROMIUM` env var if several builds coexist in the store), and the
config throws a clear error on hosts where no Nix chromium exists, so it is
safe to leave checked in next to the default config — the default
`playwright.config.js` is still what CI and non-NixOS runs pick up.

```bash
cd dashboard
npm ci
npx playwright test --config=playwright.nix.config.js   # full a11y/agentation suite
npx playwright test agentation-mount.spec.js --config=playwright.nix.config.js
```

Test artifacts land in `/tmp/pw-nix-results/` rather than the checkout.

If you need a one-off variant of this config, write it under `/tmp/` — never
inside the repo. The checkout is shared between concurrent workers: an
untracked config file dirties the tree for everyone, risks being swept into
another worker's commit, and the CI build gate expects a clean tree. The
checked-in config exists precisely so the recipe stops being re-derived per
dispatch; with it there is normally nothing to create at all.

Verification baseline: with nix chromium 151.0.7922.173 the agentation-mount
spec runs 9 passed / 0 failed.

## CI Integration

The accessibility tests run as a quality gate in the `spaxel-build` Argo WorkflowTemplate. The `a11y-test` step:

```yaml
- name: a11y-test
  template: a11y-test
  arguments:
    parameters:
      - name: version
        value: "{{steps.resolve-version.outputs.parameters.version}}"
```

**Step details:**
- Runs in parallel with `golangci-lint` (after version resolution)
- Must pass before build proceeds
- Blocks releases on accessibility violations
- No `continueOn.failed` override — failures are hard stops

## Test Files

| File | Purpose |
|------|---------|
| `tests/a11y.spec.js` | Core dashboard pages (index, live, fleet, setup, integrations) |
| `tests/a11y-dashboard.spec.js` | Remaining entry points (ambient, live, simple, simulator) |
| `tests/a11y-onboarding.spec.js` | New user onboarding flow |
| `tests/a11y-entrypoint-coverage.spec.js` | Guard: every top-level `dashboard/*.html` must appear in one of the two page specs (`dashboard/_dev/` harnesses are excluded) |
| `tests/agentation-mount.spec.js` | Agentation toolbar mounts on every entry point (import map, JS Content-Type, `#agentation-root`, shadow-root render) + guard against unwired new entry points |
| `tests/accessibility/helper.js` | Shared axe-core scanning and assertion helpers |

## Common Violations

The axe-core tests catch common accessibility issues:

- **Color contrast** — text/background contrast ratios < 4.5:1
- **Missing labels** — form inputs without accessible labels
- **Empty links** — `<a>` tags without descriptive text
- **Heading structure** — skipped heading levels (h1 → h3)
- **ARIA attributes** — missing or incorrect ARIA roles/properties

## CI Execution

GitHub Actions are disabled across all repos — all CI runs on Argo Workflows (iad-ci). The accessibility tests are wired into the `spaxel-build` Argo WorkflowTemplate and run automatically on every build.

**Workflow:** `spaxel-build`  
**Namespace:** `argo-workflows`  
**Cluster:** `iad-ci`

## Acceptance Criteria

The CI gate passes when:
- ✅ All accessibility tests pass (zero violations)
- ✅ No WCAG 2.1 AA violations on any tested page
- ✅ No regressions in previously accessible components

**Failed gate:** Any accessibility violation blocks the release and must be fixed before deployment.

## Fixing Violations

When accessibility tests fail:

1. **Check the CI logs** — axe-core provides detailed violation reports
2. **Run locally** — reproduce with `npm run test:a11y --headed` (on NixOS: `npx playwright test --config=playwright.nix.config.js`, see [Running locally on NixOS](#running-locally-on-nixos))
3. **Fix the issue** — update HTML/ARIA attributes in dashboard files
4. **Verify** — re-run tests locally
5. **Commit** — push the fix and re-trigger CI

## Documentation Updates

This gate enforces accessibility at the CI level. Complement with:
- Manual testing with screen readers (NVDA, JAWS)
- Keyboard-only navigation testing
- Color-blind accessibility checks

## Resources

- [WCAG 2.1 AA Quick Reference](https://www.w3.org/WAI/WCAG21/quickref/)
- [axe-core Rules](https://www.deque.com/axe/core-documentation/)
- [Playwright Accessibility Testing](https://playwright.dev/docs/accessibility-testing)
