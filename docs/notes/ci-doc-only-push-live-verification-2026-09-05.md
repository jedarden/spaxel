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

Recorded 2026-09-05 ~07:00 UTC, observation window 06:29–07:00.

### Push timeline

From `git reflog show origin/main` (local ref update times, UTC). The three pushes
are **three separate `update by push` events**, so the docs push is an isolated
push — not one bundled push whose diff merely contained a docs file.

| push (UTC) | commit | changed paths | live `ignored_path()` verdict |
|---|---|---|---|
| 06:29:30 | `a54c9951` (this test) | `docs/notes/ci-doc-only-push-live-verification-2026-09-05.md` | **ignored** — first rule `docs/` prefix; also matches `%.md$` |
| 06:30:09 | `79f3f286` | `firmware/main/{main.c,watchdog.c,watchdog.h}` | trigger — `firmware/main/**` |
| 06:31:48 | `bf40f1b6` | `VERSION` | trigger — `VERSION` |

### Sensor outcomes

Workflow attribute `events.argoproj.io/action-timestamp` (ms epoch) → UTC.

| action-ts (UTC) | workflow | trigger | latency from its push |
|---|---|---|---|
| — | **none** | — | docs push: no workflow within its 39 s clean window |
| 06:30:20.686 | `spaxel-e2e-jcfxc` | spaxel-e2e | +11.7 s from the firmware push |
| 06:32:13.355 | `spaxel-e2e-ftrdn` | spaxel-e2e | +25.4 s from the bump push |
| 06:32:13.365 | `spaxel-build-bqlsr` | spaxel-build | +25.4 s from the bump push |

Observed latency band in this window: **+4.8 s to +25.4 s** (tightest: push `0d5e6dad`
05:46:31 → pair 05:46:35.8). A docs-push trigger would have appeared by ~06:30:00 at
the latest. Nothing did.

### Acceptance criteria

| AC | Result |
|---|---|
| Test commit touching only documentation | `a54c9951` — one file, `docs/notes/*.md`, nothing else |
| Argo Events sensor shows no workflow triggered | zero spaxel workflows with action-ts in [06:29:30, 06:30:09) |
| No `spaxel-build` workflow run in iad-ci | none attributable to the docs push (see attribution below) |
| VERSION file unchanged | `0.2.173` → `0.2.173` by the docs push; `0.2.174` came only from the firmware build's bump `bf40f1b6` |
| declarative-config deployment pin unchanged | live `spaxel` + `spaxel-sim` Deployments (ardenone-cluster ns `spaxel`) still `docker.io/ronaldraygun/spaxel:0.2.24` before and after |
| No new `ronaldraygun/spaxel` image tag | no VERSION bump ⇒ no tag could be minted for the docs push; `0.2.174` (if minted) belongs to the firmware build |

### Code-level proof, independent of timing

The live predicate `ignored_path()` (`docs/build-path-filter-spec.md` §5.1) returns
true for this commit's only path on its **first** rule (`docs/` prefix) and again on
`%.md$` — two independent rules would have to fail for this push to build. §4.1
confirms `spaxel-e2e` is submitted on the same `conditions: spaxel-push`, so a
filtered push suppresses **both** triggers: the docs push produced neither.

### Attribution caveat

Workflow specs carry only `git-repo` / `branch` / `image-repo` — no SHA — so
workflow→push attribution is timing-based. `jcfxc` is a lone e2e with no build
partner; its latency is +50.7 s from the docs push (outside the whole observed band)
and +11.7 s from the firmware push (inside it). It belongs to the firmware push.

### Incidental findings, out of scope here

1. The firmware push `79f3f286` produced an e2e but **no `spaxel-build` partner**
   despite `firmware/main/**` being in the build trigger set — a lone-e2e shape this
   sensor should not produce under a shared predicate. (Same shape a docs-push leak
   would have had to produce, which is why latency had to settle it.)
2. §4.1's second non-path guard — "commits authored by `Argo Workflows CI` (the
   version auto-bump) are dropped to prevent a cascade loop" — is **inert live**:
   bump commits are authored `jedarden <github@jedarden.com>` (the org-wide git
   identity), and `bf40f1b6` did fire a pair. No bump ladder ran only because spaxel
   CI is red and the bump step never executes. Worth a follow-up bead if the build
   ever goes green.

### Second data point

This section was itself pushed as a **second doc-only commit** (same path set, same
two ignore rules). Its observation is recorded in bead `spaxel-3c9854cd`'s notes
rather than in a third commit, to avoid an infinite regress of append-only notes.
