# Spaxel Dashboard

## Running Tests

### Unit Tests (Jest)

```bash
npm test
```

### Accessibility Tests (axe-core + Playwright)

```bash
# First-time setup: install browsers
npx playwright install --with-deps chromium

# Run accessibility gate
npm run test:a11y
```

The accessibility test loads each dashboard page (`index`, `live`, `fleet`, `setup`, `integrations`) via a local static server and asserts zero WCAG 2A/2AA violations using `@axe-core/playwright`. CI fails the build if any violation is introduced.

## CI Integration (Argo Workflows)

Add the following step to the `spaxel-build` WorkflowTemplate before the container build:

```yaml
- name: a11y-gate
  container:
    image: node:20-bookworm-slim
    command: [sh, -c]
    args:
      - |
        cd dashboard
        npm ci
        npx playwright install --with-deps chromium
        npm run test:a11y
    resources:
      limits:
        memory: 512Mi
        cpu: "1"
```
