# amd64 build verification — 2026-09-04

Bead: `spaxel-a6e50186` ("Verify amd64 build completes without idf.py
set-target errors"). Workflow observed: **`spaxel-build-84smx`** on `iad-ci`
(version 0.2.160, image `ronaldraygun/spaxel`), started 13:07:44Z, terminal
13:35:39Z.

## Answer up front

**The amd64 ESP-IDF firmware leg — the thing this bead is about — completed
cleanly.** `idf.py set-target esp32s3` ran with **zero errors**, the full
1,094-object build succeeded, the size gate passed, and both firmware
artifacts were published to GitHub Release `v0.2.160`. The "cache-bust
mechanism works functionally" question is therefore answered **yes**: nothing
in the firmware path is broken.

The workflow's terminal phase, however, is **`Failed`**, not `Succeeded`, and
that must be stated plainly. Both failures are outside the firmware leg:

| Leg | Phase | Why |
|---|---|---|
| `acceptance-test` | Failed (exit 1, 12m02s) | Explicitly `continueOn`'d in the template — a tolerated chronic red, not a gate |
| `build` (docker-build) | Failed (exit 1, **17s**) | Parallel-group race, see [docker-build failure](#docker-build-failure) below |

## AC-by-AC results

| # | Criterion | Result |
|---|---|---|
| 1 | Monitor the amd64 workflow to completion | **PASS** — watched `spaxel-build-84smx` 13:07:44Z → 13:35:39Z (27m55s); firmware-build pod logs streamed live to beat `podGC: OnPodCompletion` |
| 2 | Final phase is `Succeeded` | **FAIL at workflow level** — terminal phase is `Failed`. The `build-firmware` step itself is `Succeeded`; the two failing legs are the `continueOn`'d acceptance red and the docker-build 17s race (neither is the firmware build) |
| 3 | No idf.py set-target errors | **PASS** — `Executing action: set-target` → `Set Target to: esp32s3, new sdkconfig will be created.` → clean cmake configure, no error output anywhere in 1,451 log lines |
| 4 | Firmware image built correctly | **PASS** — app image 0x122a70 = **1,190,512 B** against a 0x1f0000 (2,031,616 B) app partition, **841,360 B (41%) free**; merged offset-0 OTA image `Wrote 0x142a70 bytes` = **1,321,584 B**; both uploaded to Release `v0.2.160` at 13:35:28Z and byte-identical to the log figures |
| 5 | Record build duration + warnings | **PASS** — see tables below; exactly **one** warning in the whole log |

## Build duration

Workflow `spaxel-build-84smx`: 13:07:44Z → 13:35:39Z = **27m55s** total.

| Step | Duration | Phase |
|---|---|---|
| resolve-version | 11s | Succeeded (version 0.2.160, should-build=true) |
| lint | 3m52s | Succeeded |
| a11y-test | 2m43s | Succeeded |
| go-test | 6m35s | Succeeded |
| timing-benchmark | 4m54s | Succeeded |
| acceptance-test | 12m02s | **Failed** (continueOn — tolerated) |
| firmware-test | 38s | Succeeded |
| **build-firmware (amd64 ESP-IDF)** | **2m53s** | **Succeeded** |
| build (docker-build) | 17s | **Failed** — see below |

The 2m53s firmware figure covers the whole pod: toolchain setup, depth-1
clone from Forgejo, pip install of `idf-component-manager>=2,<3` (the v5.2
image bundles 1.x, which cannot read the committed 2.0-format
`dependencies.lock`), `set-target`, the 1,094-object ninja build, the
`check_sizes.py` partition gate, `esptool merge_bin`, and the `gh release`
upload. The compile portion is most of it.

## The one warning

```
firmware-build.log:128
warning: user value 150 on the int symbol ESP_TASK_WDT_TIMEOUT_S
(defined at /opt/esp/idf/components/esp_system/Kconfig:441) ignored due to
being outside the active range ([1, 60]) -- falling back on defaults
```

Side finding: `firmware/sdkconfig.defaults:77` sets
`CONFIG_ESP_TASK_WDT_TIMEOUT_S=150` at HEAD. ESP-IDF silently drops
out-of-range symbols from `sdkconfig.defaults`, so this line is a **no-op**
— the task watchdog runs at the default (5s), not 150s. Not fixed here: that
file has concurrent uncommitted edits from another worker in this checkout,
and a config-semantics change deserves its own bead. Recorded, not edited.

No other warnings. No errors — the only `error` hits in the log are source
filenames (`error.c.obj`, `esp_tls_error_capture.c.obj`) and Argo's final
`error=<nil>`.

## Artifact confirmation

Release `v0.2.160` — "Spaxel Firmware v0.2.160", published 2026-09-04T13:35:27Z:

| Asset | Size | Matches log |
|---|---|---|
| `spaxel-firmware-0.2.160-merged.bin` | 1,321,584 B | = 0x142a70 ✓ |
| `spaxel-firmware.bin` | 1,190,512 B | = 0x122a70 ✓ |

Download URL as reported by the leg:
`https://github.com/jedarden/spaxel/releases/download/v0.2.160/spaxel-firmware-merged.bin`

Note the `dependencies.lock` NOTICE: the leg recreates the 1.0.0-format lock
as 2.0.0 inside the container on every run (per the
[Firmware size-AC control-build playbook](amd64-build-cache-analysis.md)
notes on the idf-component-manager pin). It is a per-run in-container
rewrite, not a repo change — nothing was committed back.

## docker-build failure

`build` (docker-build) died **17 seconds** after starting, exit 1. Too short
to build an image, and no logs survive (`podGC: OnPodCompletion`), so this is
inference — but the mechanism is pinned down by three facts:

1. **It starts in parallel with `build-firmware`.** Both pods started at
   exactly 13:32:36Z (step group 5 runs them concurrently).
2. **The Dockerfile fetches the firmware from GitHub Releases** at build
   time — `Dockerfile:10-26`, stage `firmware-fetcher`:
   `curl -fsSL .../releases/download/v${VERSION}/spaxel-firmware-${VERSION}-merged.bin`.
   `-fsSL` fails hard on a 404.
3. **The release did not exist yet.** `build-firmware` published `v0.2.160`
   at 13:35:27Z — 2m34s *after* docker-build had already exited.

So docker-build's `firmware-fetcher` stage asked for an artifact its sibling
step had not published yet, and failed fast. This is a **deterministic
parallel-group race**, not flakiness: it only passes when the Go layers are
fully cache-hits and build-firmware happens to win by more than the fetch
lead time. It is the same failure shape as the already-documented
docker-e2e "firmware-fetch 404" red (CI diagnosis notes) — the decoupling at
b25dc130 moved the firmware out of the Docker build but left the dependency
on a *sequential* publish, while the template keeps the two legs in one
parallel group.

**Not fixed here.** The fix is a `depends`/step-group reorder in the
`spaxel-build` WorkflowTemplate, which lives in `jedarden/declarative-config`
— outside this repo and outside this bead's scope. Filed here as the record;
whoever owns the template change needs only: make `build` depend on
`build-firmware` completing (or move the two into sequential groups).

## Corroborating context from the same window

The other two `spaxel-build` runs visible in the cluster (both 12:34Z, for
0.2.159) died earlier at `go-test` (exit 1 / activeDeadline) and never
reached the build group — so this run is the only recent observation of the
build legs, and docker-build's health before today is not establishable from
the retained workflow history. `update-declarative-config` has not run in
any retained run, so no image tag has been bumped in declarative-config from
this window.

## Method note

Evidence was captured live because `podGC: OnPodCompletion` deletes pods the
instant a step finishes: a poller watched for the `firmware-build` pod and
streamed `kubectl logs -f` to a local file for the life of the step (1,451
lines). All quoted log lines carry their original line numbers. The raw
capture was ephemeral and is not committed; the decisive lines are quoted
above and the artifacts are durable on GitHub Releases.
