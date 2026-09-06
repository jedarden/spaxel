# spaxel-build pipeline — operating runbook

Status: current as of 2026-09-06. Live template revision `75948173`
(argo-workflows-ns-iad-ci), last template sync 2026-09-06T13:44:09Z.

This file exists because the `spaxel-build` WorkflowTemplate's operating
state — credential wiring, gating, failure owners, evidence conventions —
lived only in scattered bead notes. Eight separate junk beads
(spaxel-8e7b6135, spaxel-9a872815, spaxel-ca7e574b, spaxel-36bcfae8,
spaxel-4657d78a, spaxel-dc42d07f, spaxel-1c0bf8c7, spaxel-2aea08a4) each
re-derived parts of it from scratch. This is the durable copy. When a fact
here and the live template disagree, the template wins — re-verify and fix
this file.

Only secret **names** appear below. Values live in the cluster and OpenBao,
never in this repo.

## 1. Where the template lives and how it changes

- Repo: `jedarden/declarative-config`, file
  `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`.
- Synced into `iad-ci` namespace `argo-workflows` by ArgoCD app
  `argo-workflows-ns-iad-ci`.
- **Never mutate it with `kubectl`.** Read-only `get`/`describe` is fine;
  every change goes through declarative-config → commit → push → ArgoCD sync.
- There is **no separate amd64-only template.** An earlier fork named
  `spaxel-build-amd64` existed for one day (2026-09-01 → 2026-09-04) and was
  deleted (declarative-config `a979e063` added the parameter, `e30e76f0`
  deleted the fork manifest). amd64-only is now the `platforms` parameter —
  do not reintroduce the fork.
- Workflow parameters: `git-repo=jedarden/spaxel`,
  `image-repo=ronaldraygun/spaxel`, `branch=main`,
  `platforms=linux/amd64,linux/arm64`.

## 2. Step sequence and gating

The template entrypoint is `build` and it uses **sequential step groups**
(not a DAG). A step that fails without `continueOn` halts the whole chain,
so a red leg early in the sequence starves every leg after it — a golangci
failure means go-test, acceptance, firmware and the image push never run at
all.

| # | Step group | Image | Notes |
|---|---|---|---|
| 1 | resolve-version | golang:1.25-bookworm | always runs; decides `should-build` |
| 2 | golangci-lint + a11y-test | golang:1.25-bookworm / node:20-bookworm | |
| 3 | go-test + timing-benchmark | golang:1.25-bookworm | |
| 4 | acceptance-test | golang:1.25-bookworm | **only leg with `continueOn: {failed: true}`** — its red does not block downstream |
| 5 | firmware-test | gcc:16.1.0-trixie | host-side test build, no ESP-IDF |
| 6 | build-firmware | espressif/idf:v5.2 | size gate + draft release publish |
| 7 | build (docker-build) | docker:29.7.2-dind | multi-arch image push |
| 8 | update-declarative-config | alpine/git | `activeDeadlineSeconds: 120` |

Groups 2–8 are each gated on
`when: "{{steps.resolve-version.outputs.parameters.should-build}} == true"`.
When the gate is false the nodes report **Skipped** with
`when 'false == true'` — that is the normal, healthy shape of a docs-only
push, not a failure.

Older bead notes use a `group[1]..group[6]` shorthand for the same
sequence. Mapping, so those notes stay readable:

| Note shorthand | Live step groups |
|---|---|
| group[1] | golangci-lint + a11y-test |
| group[2] | go-test + timing-benchmark |
| group[3] | acceptance-test |
| group[4] | firmware-test |
| group[5] | build-firmware **and** docker-build — two consecutive groups |
| group[6] | update-declarative-config |

## 3. resolve-version — the substantive-change gate

- Shallow anonymous clone (`--depth 2`) of
  `https://git.ardenone.com/jedarden/spaxel.git`. The repo `.git` is
  ~650MB; a full clone blows the container budget. No credential is needed
  to read.
- The tip's changed paths are filtered with
  `grep -vE '^docs/|^\.beads/|^\.needle|\.md$|^LICENSE$|^\.gitignore$'`.
  If nothing survives, `should-build=false` and groups 2–8 are Skipped.
  **Consequence:** a docs/.beads-only push produces no image and no
  firmware release, and no "all-green" workflow can exist for such a tip.
- If the tip already changed `VERSION`, that value is used as-is.
  Otherwise the patch version is auto-bumped, committed as
  `ci: auto-bump version to X.Y.Z` with identity
  `jedarden <github@jedarden.com>`, and pushed to Forgejo:
  `git push "https://x-token:${GIT_TOKEN}@git.ardenone.com/jedarden/spaxel.git" HEAD:main`.
- Recent examples: `32da3dc7` (0.2.181, 2026-09-06), `485391ff` (0.2.182).

## 4. Credentials per container

| Env | Secret / key | Present in | Used for |
|---|---|---|---|
| `FORGEJO_TOKEN` | `forgejo-webhook-token` / `token` | every container | Forgejo (git.ardenone.com) auth |
| `GIT_TOKEN` | `forgejo-webhook-token` / `token` | resolve-version | push of the auto-bump commit |
| `GH_TOKEN` | `github-webhook-secret` / `token` | every container | **GitHub only** — `gh` release ops and the BuildKit build secret |
| — (volume) | `docker-hub-registry` / `.dockerconfigjson` → `config.json` | docker-build, mounted at `/root/.docker` | Docker Hub push auth |

Two rules that have both caused real incidents:

- **Forgejo git auth is username `x-token` + `forgejo-webhook-token`.** Two
  wiring patterns are in use: URL-embedded (resolve-version's `git push`)
  and an inline credential helper (update-declarative-config, below).
- **`github-webhook-secret` is rejected by git.ardenone.com** (since
  2026-09-03). It is a GitHub token; wiring it into a Forgejo remote fails
  auth. Never reintroduce it as a git credential — it is for `gh` and
  BuildKit only.

update-declarative-config's credential-helper pattern (the reusable form):

```
GIT_CONFIG_COUNT=1
GIT_CONFIG_KEY_0=credential.helper
GIT_CONFIG_VALUE_0=!f() { test "$1" = get && echo "username=x-token" && echo "password=$FORGEJO_TOKEN"; }; f
FORGEJO_TOKEN  ← secretKeyRef forgejo-webhook-token / token
```

## 5. Firmware leg (build-firmware)

Image `espressif/idf:v5.2`.

- **Component-manager pin.** `dependencies.lock` is component-manager lock
  format 2.0.0, written by a 2.x manager; the manager bundled with this
  image is 1.x, which only parses format 1.0.0 and aborts at the first
  invocation. The script therefore runs
  `python -m pip install --no-cache-dir "idf-component-manager>=2,<3"`
  immediately after `. $IDF_PATH/export.sh`, before any `idf.py`.
  (declarative-config `e35c46df`; verified end-to-end in spaxel-717ca012 —
  bundled 1.5.1, installed 2.5.2, committed lock honored pin-for-pin.)
- **Size gate.** `check_sizes.py --offset 0x8000 partition --type app` must
  pass. Size ACs are judged against a control build from the same tree, not
  against a stale figure in a plan doc.
- **Release publish.** The leg creates a GitHub release targeting the built
  commit (`gh release create … --target "$(git rev-parse HEAD)"`) and then
  runs `gh release edit "$RELEASE_TAG" --draft=false`. The publish step
  exists because v0.2.148–v0.2.180 all sat as **draft**, and draft assets
  404 to the anonymous/authenticated fetch docker-build uses — every one of
  those releases was unbuilt from the image's point of view. If a firmware
  leg succeeds but the release is still draft, the pipeline is not actually
  green.

## 6. Image leg (docker-build)

Image `docker:29.7.2-dind`, buildx with a `docker-container` driver
(`multiarch-builder`).

- **`DOCKER_CONFIG` must be repointed to a writable dir.** The registry
  credential arrives as a Secret volume at `/root/.docker`, and buildx
  writes builder state under `$DOCKER_CONFIG/buildx` — `buildx create` dies
  with `mkdir /root/.docker/buildx: read-only file system` (the EROFS red
  owned by spaxel-ed71307f). The script does `export
  DOCKER_CONFIG=/tmp/docker-config`, `mkdir -p`, then copies
  `/root/.docker/config.json` in.
- **Draft-release fetch.** Firmware is fetched from GitHub Releases during
  the build; draft assets 404, so `GH_TOKEN` is handed to BuildKit as
  `--secret id=gh_token,env=GH_TOKEN` and surfaces at
  `/run/secrets/gh_token` inside the firmware-fetcher stage
  (spaxel-side fix `c0b80cc3`).
- Build is `--platform={{workflow.parameters.platforms}}` with two tags,
  `:{{version}}` and `:latest`, `--push`, and a registry cache at
  `:buildcache` (`mode=max`). Note `:latest` here is the template's own
  runtime tagging of the image it just built — the org rule against
  `:latest` governs manifests *we* write, which always pin a version.
- **The last command is `docker buildx imagetools inspect
  <image-repo>:<version>`** under `set -e`. That makes the node phase
  self-proving: `Succeeded` means the push resolved registry-side. This
  matters because podGC never preserves pods (§8) — node phase plus a
  registry-side digest check is the evidence, not logs.

## 7. update-declarative-config (group[6]) — known not landing

The leg clones declarative-config (credential helper above) and seds the
new version into `k8s/ardenone-cluster/spaxel/deployment.yml` and
`nixos/bench/modules/mothership.nix`. It has **never landed a pin bump**:
the bench still pins `0.2.24` (declarative-config `adb644cf`, 2026-08-07),
while the pipeline is at 0.2.18x. The work is currently **unowned** — no
open bead carries it. Until someone owns it, assume the cluster image ref
does **not** follow `main`, and do not treat a green pipeline as "deployed".

## 8. Evidence conventions

The cluster is hostile to after-the-fact forensics. Design every
verification around it:

- **podGC is `OnPodCompletion`** — pods are deleted the moment they finish.
  To capture logs you must stream `kubectl logs -f -c main <pod>` from pod
  birth. After completion, the surviving evidence is node phases and
  whatever the leg pushed/fetched registry- or git-side.
- **TTL pruning deletes whole Workflows** (workflowDefaults:
  `secondsAfterCompletion: 3600`, `secondsAfterFailure: 7200`,
  `secondsAfterSuccess: 1800`). A run older than ~1–2 hours may have no
  object at all. Record the workflow **name** and per-node phases in the
  bead at the time.
- Per-node failure detail:
  `kubectl --server=http://traefik-iad-ci:8001 get workflow <name> -n argo-workflows -o json`, then walk `.status.nodes` for `phase`/`message`.
  A template whose overall phase is `Failed` may still have green legs —
  judge per node.
- **Firmware-size ACs** are settled with a same-tree single-line-flip
  control build, never against a figure quoted in a stale doc.
- **Image pushes** are verified registry-side
  (`docker manifest inspect ronaldraygun/spaxel:<tag>`; the Docker Hub
  repos are private and return 404 to unauthenticated reads — 404 is an
  auth wall, not absence). One multi-arch digest covering amd64+arm64 is
  the healthy shape.
- **Go gates run locally** from `mothership/` only (single Go module;
  `go.work` selects `./mothership`). `go vet` then `go test ./...`; two
  load-sensitive perf failures are known pre-existing under load. A
  `git archive` sandbox needs `go.work` **and** `go.work.sum` copied in or
  you get fake `missing go.sum entry` errors.
- A `verification-failed` label from the acceptance-suite leg is **never**
  evidence about the e2e package — that CI leg has never been green.

## 9. Known-good runs

- **Full pipeline on a substantive tip** (commit-identified): tip
  `e64d35cc`, 2026-09-06 06:45:52Z, auto-bump push `32da3dc7`
  (0.2.180 → 0.2.181). Groups through build-firmware green; the only red
  was docker-build EROFS, since fixed. The workflow object itself is
  TTL-pruned and its name was never recorded — the evidence chain is the
  bump commit, the recorded node phases, and the registry timestamps.
- **`spaxel-build-ed71307f-bp8xq`** (docker-build-entrypoint probe,
  2026-09-06 13:59:41 → 14:05:33Z): docker-build node `Succeeded` on
  attempt 0; its final command under `set -e` was the imagetools inspect.
  Push corroborated registry-side: `:0.2.181` and `:latest`, one
  multi-arch digest, both arches.
- **`spaxel-build-1c06e46d-chjqg`** (2026-09-06 16:48:23 → 16:48:46Z):
  resolve-version `Succeeded`, everything else `Skipped` on
  `when 'false == true'` — the canonical look of a non-substantive tip.
- There is **no single post-fix all-green full run**, because every
  subsequent `main` tip is docs/.beads-only and skips the pipeline by
  design. Do not manufacture one just to have a name to cite: it would
  push a production image and a draft release for nothing.

## 10. Red owners — attribute, don't re-fix

| Red | Owner | State |
|---|---|---|
| golangci-lint on mothership | spaxel-a2e3425d, umbrella spaxel-20f9f00f | closed / deferred |
| a11y-test npm lockfile | spaxel-be6766b4 | closed |
| Kaniko executor pin (historical; template now runs docker:29.7.2-dind) | spaxel-67dfcf40 | closed |
| idf-component-manager lock-format failure | spaxel-717ca012 | closed (declarative-config `e35c46df`) |
| docker-build draft-404 + EROFS | spaxel-ed71307f | closed |
| Pipeline evidence / green-run proof | spaxel-1c06e46d | closed |

When a leg goes red, find its owner in the bead store before touching the
template. Re-fixing owned reds is how the duplicate junk beads listed in
the intro happened.

## 11. Verification recipes

```bash
# Live template (read-only; never mutate)
kubectl --server=http://traefik-iad-ci:8001 \
  get workflowtemplate spaxel-build -n argo-workflows -o json

# Recent runs (labels are unreliable — filter by name prefix)
kubectl --server=http://traefik-iad-ci:8001 get workflows -n argo-workflows \
  -o json | jq '[.items[] | select(.metadata.name | startswith("spaxel-build"))]'

# Per-node phases for one run
kubectl --server=http://traefik-iad-ci:8001 get workflow <name> \
  -n argo-workflows -o json | jq -r \
  '.status.nodes[] | "\(.displayName)\t\(.phase)\t\(.message)"'

# Registry-side push proof (Docker Hub repos are private; 404 = auth wall)
docker manifest inspect ronaldraygun/spaxel:<tag>
```
