# Spaxel Dashboard

## Running Tests

### Unit Tests (Jest)

```bash
npm test
```

`npm test` runs `jest --verbose` over the co-located `js/*.test.js` suites (config: `jest.config.js` — jsdom environment, `maxWorkers: 1` for deterministic scheduling on a shared box, ~15–30 s wall time). Dependencies are locked by `package-lock.json`; always install with `npm ci`, never a bare `npm install`, so local runs match CI exactly.

### Accessibility Tests (axe-core + Playwright)

```bash
# First-time setup: install browsers
npx playwright install --with-deps chromium

# Run accessibility gate
npm run test:a11y
```

The accessibility test loads each dashboard page (`index`, `live`, `fleet`, `setup`, `integrations`) via a local static server and asserts zero WCAG 2A/2AA violations using `@axe-core/playwright`. CI fails the build if any violation is introduced.

## CI Integration (Argo Workflows)

The `spaxel-build` WorkflowTemplate (declared in `jedarden/declarative-config`, run on iad-ci) gates every substantive push on an `a11y-test` step that runs the Jest unit suite **before** the axe gate:

```yaml
- name: a11y-test
  container:
    image: node:20-bookworm
    command: [sh, -c]
    args:
      - |
        set -e
        cd /tmp
        git clone --depth 1 --branch {{workflow.parameters.branch}} \
          "https://git.ardenone.com/{{workflow.parameters.git-repo}}.git" \
          repo
        cd repo/dashboard
        npm ci
        # Unit tests (jest) before the a11y gate: cheap (~15s, sequential),
        # and a non-zero exit under `set -e` fails the step before the
        # playwright chromium download and a11y run are paid for.
        npm test
        npx playwright install --with-deps chromium
        npm run test:a11y
    resources:
      limits:
        memory: 4Gi
        cpu: "2"
```

A unit-test regression fails the step (`set -e` + `retryPolicy: OnError`, which does not retry exit-code failures), so the workflow goes red before the a11y gate and the image build ever run. The step's args above mirror the live template; the authoritative, continuously-updated documentation of the whole pipeline — step order, gating, and live-verification recipes — is [`docs/notes/spaxel-build-pipeline.md`](../docs/notes/spaxel-build-pipeline.md) (§2 step table, §9 verification runs).

