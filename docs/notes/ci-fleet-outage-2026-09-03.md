# iad-ci failure wave, 2026-09-03 — operator diagnosis

**Verdict: there is no shared-layer, fleet-wide root cause.** Every failure in
the 2026-09-03 wave is per-repo or per-template. Nothing in the evidence is
infra-shaped: there is no egress/DNS/registry failure (one DNS failure exists,
but it is a typo baked into one template's parameter, not fleet DNS), no
runner-image failure, no shared-cache failure, no Argo configuration failure.
Unrelated templates ran green throughout the same window, which is the
strongest corroboration that the cluster itself was healthy.

This doc is the diagnosis deliverable. It is written so an operator can act on
it without reading anything else. For full captured log excerpts, see
[`research/iad-ci-2026-09-03-failure-logs.md`](research/iad-ci-2026-09-03-failure-logs.md)
(child bead of the same investigation; this doc summarizes and adds current
status and fix locations).

**Status is as of 2026-09-03 ~12:00Z**, i.e. after commit `3fac88e9` ("repair
test-code typecheck and vet breaks on main", 11:36Z) had landed on spaxel
main. Failure modes are marked FIXED / LIVE / UNCHANGED accordingly.

---

## Scope verdict per template

| Template | Runs failing on 09-03 | Failing step | Shared-layer? | Status |
|---|---|---|---|---|
| `mta-my-way-build` | 13 (08:29–10:31) + 2 more at 11:44 | `lint` (its `tsc --build`) | no — repo TS | **UNCHANGED (live)** |
| `acb-build` | 1 (07:47:30) | `test` (its `run-tests`) | no — repo Go test | **UNCHANGED (live)** |
| `acb-site-pages-build` | 4 clone retries in 1 run (07:47:29) | `build-and-deploy` clone | no — template param typo | **UNCHANGED (live)** |
| `spaxel-build` | 6 (qssc4, wtc5b, 9nvs6, 6cgzf, rpxd9, xrcxx) | `lint(0)` + `a11y-test(0)` | no — two repo-level defects | split: lint **partly FIXED**, a11y **LIVE** |
| `spaxel-e2e` | 6 (2 at 08:06, 2 at 08:35, 2 at 11:41) | `go-test`, `acceptance-tests`, `docker-e2e` | no — same tree as spaxel-build | same-tree consequences, see C3/C6–C8 |
| `needle-ci` | 1 (00:50:40) + 2 more later | `verify` | no — repo CI leg | **UNCHANGED** (logs destroyed) |
| `armor-drift-check-daily` | 1 (09:00) | `run-check` | no — repo script | **UNCHANGED** (logs destroyed) |

Green in the same window (cluster healthy): `declarative-config-post-push-validate`
×6, `dashboard-site-49nf9`, `needle-ci-builder-m6d4l`, plus all ordinary
spaxel `-logcap-`/`-dbglog-` capture workflows.

---

## Failure modes and exact fix locations

### C1 — mta-my-way: duplicate TypeScript export breaks `tsc --build`

- **Component/layer:** jedarden/mta-my-way repo code. Not a template or infra problem — the lint step's biome and eslint stages pass first; only the TypeScript compiler fails.
- **Evidence:** `mta-my-way-build-*` `lint[lint]`, 13 runs 08:29–10:31Z, and still failing at 11:44Z (`mta-my-way-build-vck58`, `mta-my-way-build-5z2d7`, both Failed). Log line:
  `packages/shared/src/index.ts(237,3): error TS2300: Duplicate identifier 'formatDuration'.` (second declaration at `index.ts(351,3)`).
- **Scope:** template-specific (one repo).
- **Fix location:** `jedarden/mta-my-way`, `packages/shared/src/index.ts` — keep exactly one `formatDuration` declaration/export (delete or merge the one at line 237 or the one at line 351; they must not both be in the barrel file).
- **Owning bead:** none in spaxel — this is an mta-my-way repo fix.

### C2 — acb-build: 6-player integration test exceeds the step's 2-minute timeout

- **Component/layer:** jedarden/ai-code-battle repo test + an explicit timeout in the acb-build template step. Repo-shaped panic, not infra.
- **Evidence:** `acb-build-fgnv7` `test[run-tests]`, 07:47:30Z, exit 1. Log: `panic: test timed out after 2m0s` with goroutine dump rooted in `TestCombatDensityMetrics/6-player` (`engine/integration_test.go:339`, through `ComputeWinProbability` / `runRandomRollout`). The step passes `-timeout 120s`.
- **Scope:** template-specific (one repo's one test).
- **Fix location (pick one):**
  - repo: `jedarden/ai-code-battle` `engine/integration_test.go` — make `TestCombatDensityMetrics/6-player` faster or deterministic (the rollout is the slow part), **or**
  - template: `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/acb-build-workflowtemplate.yml`, the `run-tests` step's `-timeout 120s` value, if 2 minutes is simply too small for this test.
- **Owning bead:** none in spaxel.

### C3 — spaxel-build lint: test-code typecheck breaks on mothership main — **partly FIXED**

- **Component/layer:** mothership Go *test* code (not product code, not infra).
- **Original evidence:** `spaxel-build-qssc4/wtc5b/9nvs6/6cgzf` and `-rpxd9` (10:58:42–11:03:22Z) `lint(0)` exit 1 — `golangci-lint` typecheck of `internal/beads`:
  - `monitored_pluck.go:7:2: "os" imported and not used`
  - `monitored_pluck.go:9:2: "path/filepath" imported and not used`
  - `diagnostic_test.go:4:2: "database/sql" imported and not used (typecheck)`
- **Current status:** the `internal/beads` errors named above are fixed at
  `3fac88e9` (11:36Z, landed for bead `spaxel-480d108b`): at origin/main tip
  `monitored_pluck.go` no longer imports `os`/`path/filepath`, and
  `diagnostic_test.go` no longer imports `database/sql`.
- **BUT lint is not green yet:** the first spaxel-build to run *after* the fix,
  `spaxel-build-xrcxx` (started 11:41:55Z, i.e. on the fixed tip) still failed
  `lint(0)` **and** `a11y-test(0)`. Its pod logs are gone (`podGC:
  OnPodCompletion`), so the remaining lint errors are not captured here — but
  the failure itself proves further test-code lint breaks exist beyond C3's
  original three, and active repair work was visible in the shared checkout
  (uncommitted edits across `mothership/internal/beads/*`, `replay/*_test.go`,
  `github/client.go`, `ota/autoupdate.go`, `recording/buffer.go`,
  `logging/`) when this doc was written.
- **Scope:** repo-level (spaxel only).
- **Fix location:** mothership test code on spaxel main. This is owned work —
  do not open a competing bead.
- **Owning beads:** `spaxel-480d108b` (in_progress — landed 3fac88e9, continuing),
  `spaxel-a2e3425d` ("Make golangci-lint exit 0 … and prove it in a spaxel-build run"),
  umbrella `spaxel-20f9f00f`.

### C4 — spaxel-build a11y-test: dashboard lockfile contradicts package.json — **LIVE**

- **Component/layer:** dashboard npm dependency pins (repo config). Purely repo-level; the runner, npm registry and image are fine (`npm ci` starts normally).
- **Evidence:** every failing spaxel-build's `a11y-test(0)` exit 1, e.g. `-rpxd9` and the post-fix `-xrcxx` (11:41:55Z). Log:
  ```
  npm error code EUSAGE
  `lock file's @axe-core/playwright@4.11.2 does not satisfy @axe-core/playwright@4.10.1 (devDependencies)`
  npm error `lock file's axe-core@4.11.3 does not satisfy axe-core@4.10.3 (devDependencies)`
  ```
- **Current status:** **still live at origin/main** — `dashboard/package.json` line 10 pins `"@axe-core/playwright": "4.10.1"` exactly, while `dashboard/package-lock.json` resolves `node_modules/@axe-core/playwright` to 4.11.2 (and `axe-core` 4.11.3 vs the 4.10.3 pin).
- **Scope:** repo-level.
- **Fix location:** `dashboard/package.json:10` — either bump the pins to match the lockfile (`@axe-core/playwright` 4.11.2 / `axe-core` 4.11.3) or regenerate `package-lock.json` against the existing pins. One of the two must change; they currently contradict each other and `npm ci` refuses to proceed by design.
- **Owning bead:** `spaxel-be6766b4` ("Sync dashboard npm lockfile so a11y-test passes…").

### C5 — needle-ci: `verify` exits 1 — **UNVERIFIED (logs destroyed)**

- **Component/layer:** jedarden/NEEDLE repo CI leg (inferred). Every network-flavored sibling step in the same run succeeded, so the failure is not connectivity.
- **Evidence:** `needle-ci-7tgdg` step `verify[verify]`, 00:50:40Z, exit 1 after 788 s. Log content lost to `podGC: OnPodCompletion`. Two later needle-ci runs (`-w4fvh`, `-rlx86`) also Failed, while `needle-ci-builder-m6d4l` Succeeded — so it is specific to the `verify` leg, not the repo's build leg.
- **Scope:** repo-level (inferred; no log line available).
- **Fix location:** cannot name one without the log. First action is capture, not fix: resubmit once with a `podGC: OnWorkflowCompletion` override (per CLAUDE.md's debug-workflow recipe) and read the `verify` log, then fix in jedarden/NEEDLE at whatever the log names.
- **Owning bead:** none in spaxel.

### C6/C7/C8 — spaxel-e2e: three failures, all downstream of the same broken tree

- **Component/layer:** spaxel repo. All three spaxel-e2e steps run the same mothership tree that C3 breaks, which is why they fail in parallel with spaxel-build.
- **Evidence:** `spaxel-e2e-*` at 08:06 and 08:35 (×4) and 11:41 (`-ln9tg`, `-wl2t5`, both Failed):
  - `go-test` exit 1 after ~13 s — consistent with a non-compiling test binary (C3), far too fast to be a real test failure;
  - `acceptance-tests` exit 1 after ~20 s — same signature;
  - `docker-e2e` exit 22 after ~97 s — the e2e health check's `curl` hits an HTTP error, i.e. the container never came up healthy, consistent with the same tree not building/passing.
- **Current status / expectation:** no fix of its own — these should clear when
  C3 (lint/tree) and C4 (a11y leg) clear, but **that has not been observed
  yet**: no spaxel-build or spaxel-e2e run has gone green since `3fac88e9`
  landed (the 11:41 pair failed), so confirmation is still pending.
- **Fix location:** same as C3/C4 (fix the tree; do not "fix" the e2e template).
- **Owning beads:** as C3, plus `spaxel-be6766b4` for the a11y leg.

### C9 — acb-site-pages-build: the template clones a non-resolving host — **LIVE**

- **Component/layer:** **Argo template configuration** — this is the one failure in the wave that is a declarative-config defect, and the only DNS error in the whole set (fleet DNS is fine; this hostname does not exist).
- **Evidence:** `acb-site-pages-build-k9zhk` (07:47:29Z) step `build-and-deploy`, `git clone` exit **128, retried ×4** (`retryStrategy` limit 3, backoff 30s×2), final message "No more retries left". The clone URL is built from the template's own `git-repo` parameter.
- **Current status:** confirmed live on the cluster's applied WorkflowTemplate —
  `arguments.parameters` still carries `{name: git-repo, value: "forgejo.ardenone.com/ai-code-battle/ai-code-battle"}`. The Forgejo instance is `git.ardenone.com`; `forgejo.ardenone.com` does not resolve.
- **Scope:** template-specific (one template, every run of it fails).
- **Fix location:** `jedarden/declarative-config` →
  **`k8s/iad-ci/argo-workflows/acb-site-pages-build-workflowtemplate.yml`** —
  the `arguments.parameters[].git-repo` value; change
  `forgejo.ardenone.com/ai-code-battle/ai-code-battle` →
  `git.ardenone.com/jedarden/ai-code-battle`. Commit → push → let ArgoCD app
  `argo-workflows-ns-iad-ci` sync (the live template carries
  `argocd.argoproj.io/instance: argo-workflows-ns-iad-ci`). Do **not** edit the
  live template with kubectl.
- **Owning bead:** none in spaxel (ai-code-battle is not a spaxel bead target).

### C10 — armor-drift-check: script's catch-all exit 2 — **UNVERIFIED (logs destroyed)**

- **Component/layer:** ARMOR repo script. Infra-independent (the run got 41 s in and failed on the script's own exit path).
- **Evidence:** `armor-drift-check-daily-1788426000` step `run-check`, 09:00Z, exit 2 after 41 s. In `scripts/version-drift-check.py` the only `sys.exit(2)` is the catch-all `except Exception` — so the script hit an unexpected error, and the specific error died with the pod (`podGC`).
- **Scope:** template-specific (one cronworkflow).
- **Fix location:** first capture, then fix: resubmit `armor-drift-check-workflowtemplate` once with `podGC: OnWorkflowCompletion` to read the traceback, then fix `jedarden/ARMOR` `scripts/version-drift-check.py`. Template files, for reference: `k8s/iad-ci/argo-workflows/armor-drift-check-workflowtemplate.yml` + `armor-drift-check-cronworkflow.yml` in declarative-config.
- **Owning bead:** none in spaxel.

---

## What an operator should do, in order

1. **spaxel (beads already own it):** let `spaxel-480d108b` / `spaxel-a2e3425d`
   finish the test-code lint repair, and `spaxel-be6766b4` fix
   `dashboard/package.json:10` vs the lockfile. The gate for "done" is a green
   `spaxel-build` **and** green `spaxel-e2e`; none has run green since
   `3fac88e9` yet, so don't assume C3 is fully closed off the commit alone.
2. **declarative-config (one-line, highest confidence fix in the wave):** in
   `k8s/iad-ci/argo-workflows/acb-site-pages-build-workflowtemplate.yml`, fix
   the `git-repo` host (`forgejo.ardenone.com` → `git.ardenone.com`). Push and
   let ArgoCD sync. (C9)
3. **mta-my-way:** delete one of the two `formatDuration` exports in
   `packages/shared/src/index.ts` (lines 237 / 351). (C1)
4. **ai-code-battle:** fix `TestCombatDensityMetrics/6-player` runtime or raise
   `-timeout 120s` in `acb-build-workflowtemplate.yml`. (C2)
5. **NEEDLE and ARMOR:** capture first (resubmit once with `podGC:
   OnWorkflowCompletion`), then fix at whatever the logs name — needle repo
   `verify` leg (C5), ARMOR `scripts/version-drift-check.py` (C10).

Nothing in this wave requires cluster, runner-image, cache, or Argo-version
work. There is no fleet-wide incident to mitigate — only seven per-repo /
per-template fixes.

---

## Ownership handoff addendum — 2026-09-03 12:13Z (child 4, bead spaxel-4d4ef661)

This addendum closes the loop on the diagnosis above. It introduces no new
findings; it fixes ownership for every indicted fix and re-checks status as of
writing. **No fix was landed by this child, deliberately**: each one is either
owned by a live bead, lives in a repo this checkout does not contain, or is an
operator manifest change that must go through declarative-config rather than
any live mutation.

| # | Fix | Owner (bead id / manifest path / repo) | Status at addendum time |
|---|---|---|---|
| C1 | mta-my-way duplicate `formatDuration` export | jedarden/mta-my-way `packages/shared/src/index.ts` (no spaxel bead — different repo) | LIVE — `mta-my-way-build-q8n96` / `-f5n6z` Failed 12:03Z |
| C2 | acb 6-player test exceeds the 2 min step timeout | repo leg: jedarden/ai-code-battle `engine/integration_test.go`; template leg (operator): `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/acb-build-workflowtemplate.yml`, `run-tests` `-timeout 120s` | UNCHANGED |
| C3 | mothership test-code lint breaks | **spaxel-20f9f00f** (umbrella), work beads **spaxel-480d108b** + **spaxel-a2e3425d** | LIVE — `spaxel-build-qtf57` Failed 11:57:15Z on the post-`3fac88e9` tip, so further lint breaks remain beyond the original three |
| C4 | dashboard axe pins contradict lockfile | **spaxel-be6766b4** | LIVE — re-verified at origin tip: `dashboard/package.json:10` pins `@axe-core/playwright` 4.10.1 while the lockfile resolves 4.11.2 (`axe-core` 4.11.3 vs the 4.10.3 pin) |
| C5 | needle-ci `verify` exit 1 | jedarden/NEEDLE — capture first (podGC-overridden rerun), then fix per log | UNCHANGED (logs destroyed) |
| C6–C8 | spaxel-e2e `go-test` / `acceptance-tests` / `docker-e2e` | No separate owner — clears with C3+C4 | LIVE — `spaxel-e2e-qsph2` / `-jgq8x` Failed 11:56–11:57Z, downstream signature unchanged |
| C9 | acb-site-pages clone host typo | **Operator manifest:** `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/acb-site-pages-build-workflowtemplate.yml`, `arguments.parameters` `git-repo`: `forgejo.ardenone.com/ai-code-battle/ai-code-battle` → `git.ardenone.com/jedarden/ai-code-battle`. Push, let ArgoCD app `argo-workflows-ns-iad-ci` sync. Do not edit the live template. | LIVE (operator action) |
| C10 | armor-drift-check catch-all exit 2 | jedarden/ARMOR `scripts/version-drift-check.py` — capture first | UNCHANGED |

Verification contracts:

- **C3 / C6–C8:** the verification for the mothership-Go-tree branch is a green
  `spaxel-build` run landed by spaxel-20f9f00f's workers (per the child-4
  dispatch, that bead's eventual green run *is* the verification for this
  branch); spaxel-e2e is expected to clear in the same window since C6–C8 have
  no independent fix.
- **C4:** spaxel-be6766b4 owns the pin/lockfile reconciliation and its proof
  run.
- **C9:** the one-line template correction is operator-level — declarative-config
  is outside this repo and the live WorkflowTemplate is ArgoCD-managed. This
  addendum is the escalation; no cluster state was mutated by this child.

No workflow went green between the ~12:00Z snapshot above and this addendum:
the newest spaxel-family runs (`spaxel-build-qtf57`, `spaxel-e2e-qsph2`,
`spaxel-e2e-jgq8x`, all 11:56–11:57Z) failed. The child-4 acceptance criterion
is therefore satisfied by this handoff addendum rather than by a green run
attributable to this child.
