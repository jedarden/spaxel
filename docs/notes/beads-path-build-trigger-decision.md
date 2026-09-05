# Decision: `.beads/` changes must NOT trigger spaxel builds

**Bead:** spaxel-62c1b7bf (split-child of spaxel-075ef1c4, "Identify paths that
should trigger spaxel builds")
**Decided:** 2026-09-05
**Complements:** `docs/notes/ci-doc-only-push-path-filter.md` (ADR-009 decision
3 — the live implementation record) and `docs/build-path-filter-spec.md` §4.1 /
§5.1
**Status:** Decision record. The behaviour this document specifies is already
live in the `spaxel-sensor` Lua filter (declarative-config); nothing needs to
change to comply.

---

## Decision

**NO.** Changes confined to `.beads/` must not trigger a spaxel Docker build,
image push, `VERSION` bump, or deployment. `.beads/` stays on the sensor's
ignore list permanently.

This is a decision, not a default: the alternative (building on bead churn) was
considered and rejected on all three axes the bead asked about.

## 1. Does bead tracking affect the running application? — No

Three independent grounds, each verified at the current tip rather than assumed:

1. **The image never contains `.beads/`.** The Dockerfile `COPY` list is
   `mothership/` (×2: go.mod/go.sum, then the module), `dashboard/` (for
   `go:embed`), and the two firmware binaries fetched from the CI release
   artifact. No stage copies a path that could pick up `.beads/`, and the
   runtime stage is `distroless/static` containing exactly three files
   (`/spaxel`, `/spaxel-sim`, `/firmware/spaxel-firmware-${VERSION}.bin`).
   A rebuild triggered by a `.beads/` change is therefore guaranteed to
   produce a byte-identical runtime payload — it cannot deliver anything to
   the fleet.

2. **Nothing in the built product reads `.beads/` for behaviour.** The only
   production-tree code that references the path at all is
   `mothership/internal/beads/diagnostic.go:192`, which *writes* pluck
   starvation diagnostics into `<workspace>/.beads/diagnostics/`. That package
   is imported by nothing outside itself — `git grep "internal/beads"` outside
   the package returns zero hits — so it is not wired into `cmd/mothership`.
   Even if it were, it writes to a host checkout's directory, which the
   deployed image does not have (ground 1).

3. **Git is not even the source of truth for beads.** `.beads/checkpoint/` is
   a durable *mirror* of the live SQLite store (`beads.db`), published
   automatically after each mutation (bead-rs R026). Its content describes
   workflow state — status, assignees, notes, revisions — of the development
   process, not of the product.

## 2. Does excluding `.beads/` risk missing code changes? — No, for the stated risk

The specific risk named in the bead — "missing code changes that are only
tracked in beads" — cannot occur, for two structural reasons:

- **No code lives under `.beads/`.** A functional change must land in
  `firmware/`, `mothership/`, `dashboard/`, `Dockerfile`, `go.mod`/`go.sum`,
  or `VERSION`. None of those is on the ignore list, so any push carrying a
  real change still fires the sensor.
- **The filter's skip condition is conjunctive across the whole push.** A push
  is skipped only when *every* changed path in *every* commit matches an
  ignored pattern. A mixed push — a bead checkpoint graft plus a one-line
  firmware fix — builds. Ignoring `.beads/` therefore cannot mask a code
  change that shares a commit with it; it only suppresses pushes that contain
  *nothing but* bookkeeping.

Beads *describe* work; the work itself reaches hardware only through the code
paths above. Bead metadata reaching deployed hardware is not a lost update —
it is content that was never supposed to be deployed.

**Residual risks accepted (and why):**

| Residual | Assessment |
|---|---|
| A file placed under `.beads/` that genuinely altered the build would be silently skipped | Accepted. Nothing does today, and the ignore list is prefix-matched to `.beads/` specifically — a new build-relevant file would have to be deliberately put there. The general form of this risk (prefix list vs. ownership) is already recorded in `ci-doc-only-push-path-filter.md` §Residual risk; fail-open on malformed payloads keeps the error direction over-triggering, never under-triggering |
| Bead tooling config (`.beads/config.json`, `.beads/.gitignore`) is also suppressed | Accepted, deliberate. Those configure the tracker, not the product; the same ground-1 argument applies |
| A workflow change that *should* redeploy could be expressed only as a bead note | Not a real path. Process changes in this repo land in `declarative-config` (a different repo, with its own ArgoCD sync) — never in spaxel's `.beads/` |

## 3. Operational impact — the decisive axis

Measured over the trailing 60 days at the current tip
(`2026-07-07` → `2026-09-05`, 1000 commits):

| Category | Count | Share |
|---|---|---|
| **`.beads/`-only** (would build if not ignored — no other rule catches them) | **186** | **18.6%** |
| Mixed (`.beads/` + at least one substantive path) | 237 | 23.7% |
| No `.beads/` content at all | 577 | 57.7% |

The mixed commits are unaffected by this decision — they build either way. The
186 `.beads/`-only pushes are exactly what the ignore rule absorbs. Without it,
roughly one push in five would have started a Docker build, pushed an image,
bumped `VERSION`, and redeployed the mothership **for zero image change**.

The dominant subject line in that group is
`chore(beads): record-files-only checkpoint graft for spaxel-…` — the
gitleaks-safe checkpoint-publish commit this repo's own bead workflow emits
routinely. Bead churn is not an occasional event here; it is a *byproduct of
doing any other work*.

This is the same failure ADR-009 was written to stop (its Context records
docs-only edits producing releases 0.2.20–0.2.22 and three production
redeploys, with beads-chore commits among the offenders). Two consequences make
the cost worse than a wasted CI slot:

- Every mothership redeploy is a pod restart. During the period when nodes did
  not re-establish their WebSocket after a mothership restart (ADR-004), each
  roll knocked the fleet offline until a manual power cycle — i.e. bead churn
  translated directly into lost sensing.
- The auto-bump writes a `VERSION` commit that itself fires the sensor again
  (deliberately, fail-open), consuming a second workflow slot per bead-graft
  push.

## Implementation consequence

The implementer of the path filter should change **nothing** as a result of
this bead, and should preserve three properties that already hold:

1. `.beads/` remains on the ignore list. In the §5.1 Lua this is
   `if string.sub(path, 1, 7) == ".beads/"` — the prefix length **7** is
   correct (`.beads/` is 7 characters). Off-by-one here fails silently, which
   is exactly the trap §4.1's scope note warns about.
2. The skip condition stays conjunctive (all paths, all commits). Loosening it
   to "any ignored path" would reintroduce the masking risk this decision
   rules out.
3. The filter stays fail-open: a malformed or truncated payload must build,
   never skip.

## Out of scope, but found during this analysis

`.dockerignore` does not list `.beads/`. On a long-lived working copy this
directory holds untracked runtime state (`.beads/traces/` alone was 2.0 GB on
this checkout, against 28 tracked files totalling ~14.6 MB), so a **local**
`docker build .` would ship the whole 2 GB as build context. CI is unaffected —
it builds from a clean clone containing only the tracked 14.6 MB — and image
content is unaffected either way (no stage copies it). Adding `.beads/` to
`.dockerignore` is a reasonable one-line context-size optimisation, tracked
separately; it is not required by, and does not change, this decision.

## Verification

Reproduce the measurement (run from the repo root):

```bash
git log --since="$(date -u -d '60 days ago' +%Y-%m-%d)" --name-only \
  --pretty=format:'@%H' | python3 -c "
import sys
beads_only=mixed=no_beads=0; h=None; files=[]
def flush():
    global beads_only,mixed,no_beads
    if h is None or not files: return
    b=[f for f in files if f.startswith('.beads/')]
    if len(b)==len(files): beads_only+=1
    elif b: mixed+=1
    else: no_beads+=1
for line in sys.stdin:
    line=line.rstrip('\n')
    if line.startswith('@'): flush(); h=line[1:]; files=[]
    elif line.strip(): files.append(line.strip())
flush()
print(f'beads_only={beads_only} mixed={mixed} no_beads={no_beads}')"

# No production code reads .beads/ for behaviour (only the writer below):
git grep -n '\.beads' -- mothership dashboard firmware Dockerfile
git grep -ln 'internal/beads' -- mothership | grep -v '^mothership/internal/beads/'
# (expect: no output from the second command)

# No image stage can pick it up:
grep -n '^COPY' Dockerfile
```

---

**Decision stands as written.** `.beads/` is bookkeeping; bookkeeping does not
ship.
