# spaxel — Marathon Coding Instruction

You are an autonomous developer implementing **spaxel**, a WiFi CSI-based indoor
positioning system for self-hosted homes: a Go "mothership" backend (chi router, WebSocket
CSI ingestion, fleet API), a browser dashboard (Three.js + accessibility), and ESP32
firmware (ESP-IDF C), shipped as a single Docker container. You run one iteration at a time:
pick the single best bead, implement it, prove it, commit/push, close it, and exit. The loop
restarts you for the next bead.

## Authoritative sources (read before coding)

- **Plan — the source of truth:** `/home/coding/spaxel/docs/plan/plan.md` (~4,150 lines).
  Bead descriptions reference plan sections; read the relevant section before writing code.
  If the code contradicts the plan, the code is wrong.
- **Environment:** `/home/coding/CLAUDE.md` — beads (`br`), Argo CI on iad-ci, kubectl-proxy,
  ArgoCD, ADB. spaxel has no local CLAUDE.md, so the parent applies in full.

## Working directory

`/home/coding/spaxel`

## Each iteration

### 1. Sync and find work

```bash
cd /home/coding/spaxel
git pull --ff-only || git pull --rebase   # if the branch diverged, rebase local work
br ready --limit 10                        # unblocked beads, ranked by impact-weighted score
```

The `float` column is critical-path slack: `float=0` = on the critical path, larger = more
slack. **Prefer low-float, high-priority beads** (P0 first). `br ready --limit 0` is buggy
(returns nothing) — always pass an explicit limit. If a bead was attempted before (check
`git log` for its ID), continue from the prior work rather than starting over.

### 2. Claim

```bash
br update <bead-id> --status in_progress --assignee marathon
```

### 3. Implement

1. `br show <bead-id>` — read the full description + acceptance criteria.
2. Read the referenced section of `plan.md`.
3. Read the existing source before modifying it. The repo spans: Go mothership
   (`mothership/` / `cmd/`, chi router + WebSocket ingestion + SQLite/Postgres), dashboard
   (`dashboard/`, vanilla JS + Three.js), and ESP32 firmware (`firmware/`, ESP-IDF C).
4. Write production-quality code: handle errors (no panics in library code), structured
   logging, tests for every Go package. Follow the conventions already in the file you edit.
5. Gates — all relevant ones must pass before you commit:
   ```bash
   gofmt -l . && go vet ./... && go build ./... && go test ./...   # Go mothership
   # dashboard changes: cd dashboard && npm ci && npm test && npm run build (incl. axe-core a11y)
   # firmware changes: build via the ESP-IDF path the plan specifies (simulator stubs where noted)
   ```

### 4. Commit, push, close

```bash
git add <specific paths you changed>
git commit -m "<type>(<scope>): <short summary>"   # body: key decisions + Closes: <bead-id>
git push
```

**Closing a bead — `br close` is BROKEN** (returns `Error: Query returned no rows`).
Use `br batch` instead, with a substantive reason citing commits + tests:

```bash
br batch --json '[{"op":"close","id":"<bead-id>","reason":"<commits + tests + acceptance notes>"}]'
# Expected: [op 0] ok
```

### 5. End the iteration

**One bead per iteration.** Then exit — the loop restarts you.

## Hard rules

- **The plan is the source of truth.** Genuine gaps → open a `plan-gap: <title>` bead and continue.
- **Never edit `.beads/` files directly** (issues.jsonl, beads.db). Use `br` only.
- **Never force-push. Never `--no-verify`. Never skip hooks.**
- **No GitHub Actions** (they are disabled). CI is the `spaxel-build` Argo WorkflowTemplate on
  iad-ci → image `ronaldraygun/spaxel`. **No K8s Jobs/CronJobs, no direct `kubectl apply`** —
  K8s YAML goes to `jedarden/declarative-config` via PR.
- **Single container, one exposed port (8080).** Don't split the deployment.
- **Always compile.** Never leave the repo broken. If a bead is too big to finish, implement a
  coherent slice, commit what builds + passes, and leave a TODO.

## Done

The work is complete when the open-bead queue is exhausted and the plan's phases are all
implemented. Until then, every iteration ends with a commit + push + closed bead.
