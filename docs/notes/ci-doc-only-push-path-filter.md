# CI Trigger Path Filter — Doc/Beads-Only Pushes Do Not Release

**Bead:** spaxel-6816405f (ADR-009 decision 3; prerequisite for enabling auto-update)
**Verified:** 2026-08-20
**Canonical implementation:** `jedarden/declarative-config` → `k8s/iad-ci/argo-events/spaxel-sensor.yml` (not in this repo)

## What exists

The argo-events `spaxel-sensor` carries a Lua path filter on its `spaxel-push`
dependency that drops non-functional pushes **before** a workflow is created,
so they consume no iad-ci build slot at all. This is the preferred location
from the bead — filter at the sensor, not inside `spaxel-build`.

Paths ignored (per commit, across `added`/`modified`/`removed`):

| Pattern | Rationale |
|---|---|
| `docs/**` | documentation |
| `.beads/**`, `.needle*` | bead/NEEDLE bookkeeping churn — syncs are frequent and never functional |
| `*.md` (anywhere) | root `README.md`, `PROGRESS.md`, `notes/**.md` |
| `LICENSE`, `.gitignore` | non-functional |

A push is skipped only when **every** changed path in **every** commit matches
an ignored pattern. Any single substantive path (e.g. `firmware/`,
`mothership/`, `dashboard/`, `VERSION`, `Dockerfile`) triggers the build.

The filter **fails open**: unknown/incomplete payload shapes — missing
`commits`, non-table path lists, or GitHub truncating a large push
(`size > #commits`) — all build rather than skip. A substantive release can
never be silently dropped by a malformed webhook payload.

The `.beads/` question from the bead ("should bead churn trigger builds?")
is answered by the implementation: **no**. Bead syncs are the most frequent
push category in this repo and are never functional; ADR-009's whole point is
that non-functional churn must not reach hardware.

## Complementary workflow-level skip

`spaxel-build`'s `resolve-version` step separately refuses to bump when the
diff is `VERSION`-only. The bump push itself still passes the sensor filter
(`VERSION` is not an ignored path — deliberately, fail-open) and starts a
workflow run, but that run exits without re-bumping or rebuilding the image.
Known minor cost: the bump push consumes one workflow slot. Sequence observed
2026-08-20: `341b1d8` (test(ble)) → sensor fires → workflow bumps
`VERSION` to 0.2.55 (`4b47c19`) → bump push fires sensor → run
`spaxel-build-8cjg8` completes with **no** 0.2.56.

## Verification evidence

Behavioral, from full git history + workflow records:

- **Before the filter (2026-08-07):** pure `docs/plan/plan.md` edits
  (ADR-007/008) produced 0.2.20–0.2.22 and redeployed production three times;
  beads-chore commits (`96e3c3f`, `5b88b68`, `02ae8df`, `39ef68a`) likewise
  bumped. These are the incidents ADR-009 cites.
- **After the filter:** every doc/beads-only push head produced no `VERSION`
  bump and no workflow side effect — `e36d142` (2026-08-16, beads-only),
  `d07de49` and `ad4f687` (2026-08-19, beads-only), `ca7c8d8`
  (2026-08-20, `docs/notes/` only).
- **Substantive still builds:** `341b1d8` (test(ble), 2026-08-20) bumped to
  0.2.55 within ~45 s of push; `a090412` (fix(mdns)) → 0.2.54.
- Sensor pod template carries a `restarted-at` annotation of
  2026-08-07T07:53:59Z — the filter landed the same day the incidents were
  filed, via declarative-config (ArgoCD-synced).

Workflows are GC'd quickly (TTL 30 min success / 2 h failure), so "no
workflow run" for historical pushes is established by the absence of the
bump — the durable side effect any successful run produces.

## Acceptance criteria (from the bead)

- "a commit touching only `docs/` or `.beads/` produces no workflow run, no
  `VERSION` bump, and no deployment change" — **met**, per evidence above.
- "a commit touching `firmware/` or `mothership/` still does" — **met**
  (`341b1d8`, `a090412`).

## Residual risk

The ignore list is prefix/extension-based, not ownership-based. A new
top-level non-functional artifact (e.g. a future `.github/` or editor config)
would trigger builds until added to `ignored_path` in the sensor. Fail-open
makes this the safe direction — over-triggering, never under-triggering.

---

**Status:** ✅ Implemented in declarative-config (`spaxel-sensor.yml`),
verified live 2026-08-20 from git history and workflow records. This note is
the in-repo record for ADR-009 decision 3.

The `.beads/` entry has its own decision record, with the rationale the
`.beads/` bead asked for explicitly (image-content grounds, the masking-risk
analysis, and a measured 18.6%-of-pushes operational impact):
[beads-path-build-trigger-decision.md](beads-path-build-trigger-decision.md)
(spaxel-62c1b7bf, 2026-09-05).
