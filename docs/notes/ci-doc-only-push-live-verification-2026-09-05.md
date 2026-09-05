# CI Path Filter — Live Doc-Only Push Verification (2026-09-05)

**Bead:** spaxel-3c9854cd (split-child of spaxel-e8c03eba)
**Kind:** controlled forward experiment — a *new* doc-only push observed live, not a
re-derivation from git history.
**Prior evidence:** [ci-doc-only-push-path-filter.md](ci-doc-only-push-path-filter.md)
(spaxel-6816405f, verified 2026-08-20 from history + workflow records).

## Why a second verification

The 2026-08-20 evidence is behavioral but retrospective: it establishes the filter's
effect through the *absence of a bump* on historical doc-only push heads. This run
makes the same claim forward-looking — commit, push, and watch the sensor not fire —
so the filter is re-proven against the sensor config as it exists *today*.

## Method

1. Record pre-state (below).
2. Commit this file — `docs/notes/*.md`, which matches the sensor's `docs/**` ignore
   pattern and nothing else — and push it. This commit **is** the test subject.
3. Poll for ~10 min:
   - `spaxel-build` / `spaxel-e2e` workflows in `argo-workflows` (iad-ci) whose
     `events.argoproj.io/action-timestamp` label postdates the push. That label is
     stamped by the sensor at trigger time and is present on every sensor-created
     workflow, so it attributes a run to a push event more precisely than creation
     time alone.
   - `origin/main` for a new `ci: auto-bump version` commit (the only durable
     side effect a successful build produces; workflows are GC'd at TTL 30 min).
   - `VERSION` at the origin tip.
   - the live deployment pin and the Docker Hub tag set.
4. Append the observation window's results to this file in a **second** doc-only
   commit, push, and observe that push as a second data point.

Attribution rule: other workers share this checkout and push substantive commits
independently. A workflow firing inside the window is attributed to *this* test only
if no other push landed between the last pre-test push and the workflow's
action-timestamp. Origin/main is fetched immediately before and after the test push
to draw that boundary.

Sensor state is not directly readable from here — the read-only cluster identity is
`Forbidden` on `sensors.argoproj.io` — so sensor-level claims in this note are
inferences from push→workflow correlation, which is the same evidence class the
2026-08-20 note used.

## Pre-state (captured 2026-09-05T06:25:04Z)

| What | Value |
|---|---|
| Local HEAD / origin/main | `50fc4ff3` (identical — no unpushed commits) |
| `VERSION` at HEAD | `0.2.173` (bump commit `15be18a1`, 2026-09-05T05:31:16Z) |
| Live deployment pin | `ardenone-cluster` ns `spaxel`, Deployments `spaxel` + `spaxel-sim` → `docker.io/ronaldraygun/spaxel:0.2.24` |
| Docker Hub tags | `0.2.24` resolves; `0.2.174` — `no such manifest` |
| Last sensor-fired runs | `spaxel-build-6jv2b` + `spaxel-e2e-8nszq`, action-timestamp 1788587195843/849 = 2026-09-05T05:46:35Z, both **Failed** |
| Latest push at capture | `50fc4ff3` (docs-only, `docs/firmware/*`), committed 06:16 UTC — no workflow after it |

The 05:46 pair is this run's **positive control**: the sensor was alive, wired, and
firing `spaxel-build` + `spaxel-e2e` on a substantive push 39 minutes before the test
push. (Both runs Failed — spaxel CI is red at HEAD for unrelated reasons; that a run
fails does not bear on whether it should have been created.)

## Results

Appended below after the observation window closes.
