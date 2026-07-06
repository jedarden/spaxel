# CI Accessibility Testing Guide

This document describes the accessibility (a11y) testing integration with Argo Workflows CI.

## Overview

Accessibility tests enforce WCAG 2.1 AA compliance as a CI quality gate for the Spaxel dashboard. These tests run automated checks using axe-core and Playwright, ensuring the dashboard remains accessible to users with disabilities.

**Test location:** `dashboard/tests/a11y*.spec.js`

**Test runner:** Playwright + @axe-core/playwright

**What it tests:** WCAG 2.1 AA compliance across:
- Main dashboard pages (index, live, fleet, setup, integrations)
- Onboarding flow
- Dashboard interactive elements

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
| `tests/a11y.spec.js` | Main dashboard pages (index, live, fleet, setup, integrations) |
| `tests/a11y-onboarding.spec.js` | New user onboarding flow |
| `tests/a11y-dashboard.spec.js` | Dashboard-specific interactive elements |
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
2. **Run locally** — reproduce with `npm run test:a11y --headed`
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
