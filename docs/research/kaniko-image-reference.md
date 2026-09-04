# Kaniko Image Reference for declarative-config WorkflowTemplates

**Research Date:** 2026-09-04
**Status:** Complete — reference + digests live-verified against gcr.io; includes the premise check that closes the spaxel-build pin family
**Bead:** spaxel-e06c991e (final output for umbrella spaxel-67dfcf40)

## Overview

Two findings:

1. **The complete Kaniko image reference is resolved and verified** — for both the
   upstream-latest stable release (v1.24.0) and the fleet convention (v1.23.2),
   including multi-arch index digests suitable for pinning. See
   [§ The reference](#the-reference).

2. **The premise this family was minted from is stale.** Umbrella spaxel-67dfcf40
   describes `spaxel-build`'s `docker-build` step as
   `image: gcr.io/kaniko-project/executor:latest`. That is no longer true at any
   layer: git HEAD in declarative-config, the working tree, and the live
   WorkflowTemplate in `iad-ci` all agree that the step runs
   `docker:29.7.2-dind` with buildx. **There is nothing to pin in
   spaxel-build** — see [§ Premise check](#premise-check-spaxel-build-does-not-use-kaniko).
   Children spaxel-827a1f33 (edit the template) and spaxel-34396119 (commit and
   push it) have no target as written.

## The reference

### v1.24.0 — upstream latest stable

Published 2025-05-23 (`published_at`; release body self-dates 2025-05-21).
Identified as the newest non-prerelease, non-draft release in the
already-fetched payload (`/tmp/spaxel-kaniko-releases.json`, 30 releases,
fetched 2026-09-04 04:21) and confirmed still current on 2026-09-04.

Registry-resolved and round-trip-verified against `gcr.io` on **2026-09-04**:

```text
gcr.io/kaniko-project/executor:v1.24.0@sha256:4e7a52dd1f14872430652bb3b027405b8dfd17c4538751c620ac005741ef9698
```

The tag points to an **OCI image index** (`application/vnd.oci.image.index.v1+json`),
so the pin above is the index digest and stays valid across node architectures:

| Platform | Manifest digest |
|---|---|
| linux/amd64 | `sha256:7cf94e02d5648080da34bec09de3a73326acde033cb2ca4f6fcd9ebefd6c1a6d` |
| linux/arm64 | `sha256:d29d0efc2476ead9bfcf8c6fe269681ad22a9b76192374e8fc10cc4a272c1192` |
| linux/s390x | `sha256:3bdb5b24e3d51265ac94bfe310c51b5c08b76730f12c05a239c182c873ab03b7` |
| linux/ppc64le | `sha256:c4128c85d4d5acb6dbe29a3000c13c0a09ed097896e218f99bfa298a91066e37` |

The index also carries four `unknown/unknown` attestation manifests
(`vnd.docker.reference.type: attestation-manifest`, cosign-style). These are
normal, not corruption — runtime pulls ignore them.

`gcr.io/kaniko-project/executor:latest` currently resolves to the **same index
digest** (verified 2026-09-04), so pinning v1.24.0 is a zero-delta freeze: it
changes no behavior today, it only stops the reference from drifting.

The debug and slim variants exist as separate tags (`:v1.24.0-debug`,
`:v1.24.0-slim`) with their own digests; they were not resolved here because no
spaxel-family template consumes them.

### v1.23.2 — the fleet convention

Published 2024-07-10. Registry-resolved against `gcr.io` on **2026-09-04**:

```text
gcr.io/kaniko-project/executor:v1.23.2@sha256:9e69fd4330ec887829c780f5126dd80edc663df6def362cd22e79bcdf00ac53f
```

| Platform | Manifest digest |
|---|---|
| linux/amd64 | `sha256:8a4f9af8ef55ef8bfaf4cfd7b15dc956609e14a4402efefd5fb2e49a0c06e2c8` |
| linux/arm64 | `sha256:b07687b489956cd5bc12c0c23d28cd2686f76023ddf770ae04a7ed987745a26f` |
| linux/s390x | `sha256:8d07ff139917e16e07b13db15c9b80df94d2f9eb293af2534da02d759830f2a9` |
| linux/ppc64le | `sha256:3021968fdb0ff01b7843d212e173a18688b1929d9f03347a823e2e0b6894a344` |

This is what declarative-config actually runs today: a repo-wide count on
2026-09-04 found **93 files / 100 references** to `executor:v1.23.2` and 8 to
`v1.23.0-debug`, and **zero** `kaniko-project/*:latest` anywhere.

## Which one to use in declarative-config

If a template ever needs Kaniko again, **use v1.23.2** — not because it is
newer (it is not) but because it is the fleet-wide toolchain: one shared
Kaniko version across ~93 templates is what makes a broken build bisectable.
Introducing v1.24.0 into a single template would recreate exactly the
divergence the umbrella bead complains about, one template at a time.

```yaml
# Fleet-consistent choice (matches 100 existing references)
image: gcr.io/kaniko-project/executor:v1.23.2@sha256:9e69fd4330ec887829c780f5126dd80edc663df6def362cd22e79bcdf00ac53f
```

```yaml
# Upstream-latest alternative, if a fleet-wide bump is ever coordinated
image: gcr.io/kaniko-project/executor:v1.24.0@sha256:4e7a52dd1f14872430652bb3b027405b8dfd17c4538751c620ac005741ef9698
```

Tag + digest together is deliberate and valid: Kubernetes pulls by digest and
ignores the tag, while the tag records the intent for humans and survives a
future `latest`-style re-point.

## Breaking changes and release notes

Kaniko's release cadence has collapsed: v1.24.0 (2025-05-23) is still the
newest stable 15 months later, and the only prereleases in the payload are the
ancient v1.19.0/v1.19.1. Any bump decision should assume upstream fixes arrive
rarely, not continuously.

The v1.23.2 → v1.24.0 delta, from the v1.24.0 release body:

- **No CLI or flag breaking changes are named.** The behavioral items are
  below; the rest of the release is dependency churn.
- **Security:** go-git upgraded to v5.13.1 addressing **CVE-2025-21613**
  (PRs #3380, #3440), plus golang-jwt v4 → 4.5.2 and a broad dependency
  refresh (containerd 1.7.18 → 1.7.27, grpc 1.64.0 → 1.64.1,
  golang.org/x/net 0.26.0 → 0.27.0, aws-sdk-go-v2 bumps). Relevant if the
  executor ever pulls from a hostile or MITM-able registry.
- **Fix:** prevent panic when an image name and a stage alias are identical
  (PR #3245) — only bites exotic multi-stage Dockerfiles.
- **CA roots:** `ca-certificates` source moved to Debian Bookworm (PR #3450),
  i.e. a newer default CA set for registry TLS.

## Premise check: spaxel-build does not use Kaniko

Verified 2026-09-04 at all three layers, which agree with each other:

| Layer | Evidence |
|---|---|
| declarative-config working tree | `grep 'image:' spaxel-build-workflowtemplate.yml` → no kaniko line |
| declarative-config git HEAD | same (`git show HEAD:…`); working tree clean of template edits |
| live cluster (`iad-ci`) | `workflowtemplate/spaxel-build` image set = `alpine/git`, `docker:29.7.2-dind`, `espressif/idf:v5.2`, `gcc:16.1.0-trixie`, `golang:1.25-bookworm`, `node:20-bookworm`; zero kaniko |

History explains the staleness: `git log -S kaniko` on the template shows two
commits — 7ae2dda3 added it, and **920a55ae** ("add firmware build step +
multi-arch support for spaxel (ADR-001)") replaced it with the buildx/dind
flow. The `:latest` kaniko reference existed only transiently between those
commits, before the fleet standard settled on v1.23.2.

**Consequences for the family:**

- **spaxel-67dfcf40** (umbrella, "Pin Kaniko executor image in spaxel-build
  (currently :latest)") — premise-false; the template it names no longer
  contains the reference it names.
- **spaxel-827a1f33** (update the template) and **spaxel-34396119** (commit and
  push) — no target as written; do not edit declarative-config for this.
- The org rule the umbrella cites is real and this family's spirit lives on
  elsewhere: `spaxel-build`'s own images are all pinned *except* two untagged
  `image: alpine/git` lines (101 and 429), which are bare-`:latest` references
  to a non-`ronaldraygun/*` image. That is a separate, unowned finding in
  declarative-config — recorded here so it is not lost, not claimed by this
  family.

## Provenance and how to re-verify

Digests were read from the registry, not from release notes or third-party
mirrors, and the pinned index digest was round-tripped (a `HEAD` on the digest
returns that same digest). gcr.io still serves these images anonymously, and
**ghcr.io does not host `kaniko-project/executor`** (token and manifest
requests both denied on 2026-09-04) — gcr.io is the only source, so registry
availability is worth re-checking before relying on this document after a long
gap.

```bash
# 1. Anonymous pull token
TOKEN=$(curl -s "https://gcr.io/v2/token?service=gcr.io&scope=repository:kaniko-project/executor:pull" | jq -r .token)

# 2. Resolve a tag to its index digest (read docker-content-digest off the headers)
curl -sI -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.oci.image.index.v1+json" \
  "https://gcr.io/v2/kaniko-project/executor/manifests/v1.24.0"

# 3. List the per-platform manifests inside that index
curl -s -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.oci.image.index.v1+json" \
  "https://gcr.io/v2/kaniko-project/executor/manifests/v1.24.0" \
  | jq -r '.manifests[] | "\(.platform.os)/\(.platform.architecture) \(.digest)"'

# 4. Cross-check the newest stable release (kaniko has no digest-bearing
#    release assets — releases payload has zero assets — so the registry is
#    the only digest authority)
curl -s "https://api.github.com/repos/GoogleContainerTools/kaniko/releases?per_page=10" \
  | jq -r '.[] | select(.prerelease==false and .draft==false) | "\(.tag_name) \(.published_at)"' | head -3
```
