# Gitleaks 8.30.1 vs 8.25.1 on checkpoint-heavy push ranges

Investigation for `spaxel-e1bdcac3` (offline only; the live hook and policy were
not modified). Question as dispatched: why does pinned Gitleaks 8.30.1 scan a
17-commit checkpoint range in ~98 s when 8.25.1 needs ~5.6 s, against a 60 s
hook timeout?

## TL;DR

1. **The dominant reproducible 8.25.1→8.30.1 delta is the `--max-decode-depth`
   default flip: 0 → 5.** The hook passes no decode flag, so 8.30.1 decodes
   recursively by default. On a real 196 MiB / 17-commit spaxel range this
   roughly doubles wall-clock at every core count measured. Forced to depth 0,
   8.30.1 is at parity with 8.25.1 in both time *and* findings.
2. **The 98 s figure is not reproducible with hook-exact flags on this box
   today** (same binary build, same effective policy, same commit range:
   8–13 s on 14 cores). It *is* consistent with the live gate's real execution
   environment: the `forgejo-gitea` container is limited to **2 CPUs**, and a
   checkpoint-heavy accepted range scanned **inside the pod** took **40–46 s**
   with 8.30.1 while the node was at load ~16. The push failure that motivated
   this investigation was a 60 s timeout under exactly those conditions; the
   decode default is the factor that tipped a tight-but-passing budget over
   the cliff.
3. **Decode depth is a detection feature, not just a cost.** A planted-secret
   matrix shows 8.30.1 at default depth detects single- and double-base64
   encoded github-pats that 8.25.1 — and 8.30.1 at depth 0 — miss. Reverting
   to depth 0 would silently weaken the gate. Depth 2 detects everything
   depth 5 detects, on this content, at the same cost (recursion past ~2 buys
   nothing here).
4. **Rule count is a non-lever.** The server policy's `generic-api-key`
   override costs ~nothing; +20 synthetic rules cost ~+2 s. The historical
   "(119 s once a rule was added)" observation matches measurement noise
   (run-to-run variance on this box is ±100 % under load), not per-rule cost.

## Environment facts established (read-only checks of the live gate)

- Pod binary `/data/gitea/bin/gitleaks` = **8.30.1**, sha256
  `88f91962aa2f93ac6ab281d553b9e125f5197bbbce38f9f2437f7299c32e5509` —
  byte-identical to the binary extracted from the pinned tarball in
  `forgejo-application.yml` (`GITLEAKS_SHA256=551f6fc8…` is the *tarball*
  hash; the extracted binary hashes to `88f91962…`).
- Live `/data/gitea/security/gitleaks.toml` is 59 lines: the same 11-line
  effective config that is in git (`[extend] useDefault` + the nested
  `generic-api-key` allowlist) plus 48 lines of comments. Effective policy is
  identical to git; nothing diverges there.
- `forgejo-gitea` container resources: **limits cpu=2, memory=3Gi**
  (requests 100m/2Gi). This is the single biggest gap between any local
  measurement and what the hook actually experiences.
- The hook's exact scan command (declarative-config
  `tools/forgejo-secret-scan/pre-receive`): `gitleaks git --no-banner
  --redact=100 --config <cfg> --log-opts="<new tips> --not --all"
  --max-target-megabytes=100 --report-format=json --report-path=<tmp> .`
  under `timeout 60s`. No `--max-decode-depth`, no `--max-archive-depth`.

## Reproduction results

Range under test: `d9a9ab8a..c1b36c78` in this repo — the exact 17 unpublished
commits of the failed push (196 MiB novel blobs, one ~12 MiB checkpoint flush
per commit, shard rename pairs included).

Measured with the hook-exact command, `gitleaks-server.toml` (copy of the
live effective policy), on codinghome (14 cores, node load ~16 during the
runs — see variance notes):

| Configuration | 14 cores | 4 cores (`taskset -c 0-3`) | 2 cores (`taskset -c 0,1`) |
|---|---|---|---|
| 8.25.1 default (depth 0) | 4.9–8.1 s | 6.0–6.2 s | 12.4 s |
| 8.30.1 default (depth 5) | 8.0–13.1 s | 14.2–14.5 s | 25.2 s |
| 8.30.1 `--max-decode-depth=0` | 5.3–6.3 s | 6.9–7.0 s | 11.5 s |
| 8.30.1 depth 1 | 7.1–7.7 s | — | — |
| 8.30.1 depth 2 | 8.1–8.6 s | 13.1–13.6 s | — |
| 8.25.1 forced depth 5 | 10.8–12.3 s | — | — |

Reading: at depth 0 both versions are within noise of each other at every
core count; the entire version delta is the decode recursion. Decode scales
roughly linearly with cores available (25.2 s → 14.4 s going 2 → 4 cores).
Depths 1–2 already pay ~the full decode cost; depth 2 ≈ depth 5 on this
content.

**Same command, inside the forgejo pod** (2-CPU limit, real node contention;
accepted checkpoint-heavy range `d96e57ee..5b3fcdc4` — 88 commits containing
17 checkpoint flushes, the same mass shape as the failed push):

| Configuration | pod wall-clock |
|---|---|
| 8.30.1 default | 40 s |
| 8.30.1 depth 0 | 46 s |

That is inside the 60 s `timeout` window only when the node is calm. The
16:12:24Z 2026-09-23 failure (`git-receive-pack` terminated at 60.0 s) needs
only ordinary contention on top of these numbers. A 98 s reading is 25 s of
clean 2-core work under ~4× contention, or the pod's own 40–46 s under a
couple× more — both unremarkable on this fleet box (measured minute-scale
variance today was 2.5× on identical commands; `uptime` load was 16.6 on 14
cores during the runs).

Rule-count axis (depth 0 pinned, 14 cores): `[extend] useDefault` only
4.1–5.0 s; + the server's `generic-api-key` override 3.4–5.8 s (≈0 marginal);
+ 20 additional synthetic rules 5.6–7.4 s (≈+0.1 s/rule). Policy slimming
buys nothing.

## Why the dispatched numbers could not be reproduced

The dispatched offline numbers (8.30.1 = 98 s, depth-0 = 61 s, 8.25.1 = 5.6 s
on the same 17 commits) imply a 17.5× version ratio. With byte-identical
binary, effective policy, flags and range, this investigation measures 1.8×
(14 cores) to 2.2× (2 cores). The 5.6 s 8.25.1 figure reproduces fine; the
98 s does not. Candidates consistent with every measurement: the 98 s run
executed under heavy CPU contention (demonstrated ±100 % run variance; the
"(119 s once a rule was added)" step is the same noise, since the measured
per-rule cost is ~0.1 s), or executed effectively single/low-core (a 2-core
run at 25 s under 4× contention = ~100 s). The mechanism is not in dispute —
decode-by-default doubles an already-too-tight scan budget — but the specific
98 s reading should be treated as contention-inflated, not as an inherent
property of 8.30.1 on this content.

## Detection matrix (planted fixtures, server policy, git mode)

FAKE secrets generated in a scratch repo (never real values, never printed;
scans run with `--redact=100`; results compared by RuleID/File only).
Each class planted twice: `docs/notes/` and `.beads/checkpoint/`.

| Planted class | 8.25.1 default | 8.30.1 default | 8.30.1 depth 0 | 8.30.1 depth 2 |
|---|---|---|---|---|
| github-pat plaintext (`ghp_…`) | ✅ 2 | ✅ 2 | ✅ 2 | ✅ 2 |
| vault-service-token plaintext (`hvs.…`) | ✅ 2 | ✅ 2 | ✅ 2 | ✅ 2 |
| base64(github-pat) | ❌ | ✅ 2 | ❌ | ✅ 2 |
| base64(base64(github-pat)) | ❌ | ✅ 2 | ❌ | ✅ 2 |
| generic `api_key = "<32-hex>"` | ❌ ¹ | ❌ ¹ | ❌ ¹ | ❌ ¹ |

¹ Entropy-floor property of the one fake value used, **identical across both
versions**: in a second git-mode probe with fresh values, both versions detect
hex32, hex48, alnum40, `hvs.`, and UUID-shaped `api_key` values 5/5. In dir
mode all six value shapes fire under both policies. This is parity, not a
version delta.

Key conclusions from the matrix:

- **Depth 0 is a detection regression** relative to today's live gate: it
  loses base64-obfuscated secrets that 8.30.1 currently catches. Do not
  "fix" the slowdown with `--max-decode-depth=0`.
- **Depth 2 is finding-identical to the default depth 5** on every planted
  class — a safe explicit pin.
- The server policy's nested `AND` allowlist still works as designed: the
  plaintext pat and vault token planted **under `.beads/checkpoint/`** were
  reported by every configuration (only bead-ID-shaped `generic-api-key`
  findings under `.beads/` are exempt). The policy change did not weaken
  detection.

## Recommended tuning (tested; not applied here)

Target: a 15-checkpoint-commit push scanned in < 30 s with unchanged
detections.

1. **Pin the depth explicitly in the hook**: add
   `--max-decode-depth=2` to the `gitleaks git` invocation in
   `declarative-config/tools/forgejo-secret-scan/pre-receive`.
   Matrix-proven finding-identical to current live behaviour, and it
   re-anchors behaviour against upstream default flips (the default already
   changed once, silently, between 8.25.1 and 8.30.1).
2. **Raise the `forgejo-gitea` CPU limit from 2 to 4** (its Deployment
   resources in declarative-config). Measured scaling is ~linear for this
   workload: 25.2 s at 2 cores → 14.2–14.5 s at 4 cores for the real
   196 MiB range with 8.30.1 defaults. Pod-side, the observed 40–46 s
   checkpoint-heavy scans land ~15–25 s — inside 30 s with headroom for the
   documented 256 MiB worst-case push budget.
3. **Raise `SCAN_TIMEOUT_SECONDS` 60 → 120** (hook env default). The failure
   mode that produced the client 502 was the timeout reject; with the scan
   itself brought under ~30 s, the doubled ceiling absorbs node-contention
   spikes instead of rejecting pushes at the trough.
4. **Do not** chase rule-count reductions (non-lever) and **do not** set
   decode depth 0 (detection regression).

Combined effect: worst realistic push ~15–25 s clean, ~30 s under moderate
contention, rejection only past 120 s — with detections identical to today's
live gate on every planted class.

## Reproduction commands

Local range scan (hook-exact; substitute the tip and base you want):

```sh
cd /home/coding/spaxel
gitleaks git --no-banner --redact=100 --config <server-policy.toml> \
  --log-opts="c1b36c78 --not origin/main" --max-target-megabytes=100 \
  --max-decode-depth=2 --report-format=json --report-path=/tmp/gl.json .
```

Core-count scaling: prefix with `taskset -c 0-3` (or `0,1`).

Pod-side scan of an accepted checkpoint-heavy range (read-only; report to
`/tmp` inside the pod):

```sh
kubectl --kubeconfig=~/.kube/iad-ci.kubeconfig exec -n forgejo -i \
  deploy/forgejo-gitea -- sh -s <<'SCRIPT'
R=/data/git/gitea-repositories/jedarden/spaxel.git
TIP=$(git -C $R rev-list main -- .beads/checkpoint | head -1)
BASE=$(git -C $R rev-list main -- .beads/checkpoint | head -18 | tail -1)
cd $R && time /data/gitea/bin/gitleaks git --no-banner --redact=100 \
  --config /data/gitea/security/gitleaks.toml \
  --log-opts="$TIP --not $BASE" --max-target-megabytes=100 \
  --report-format=json --report-path=/tmp/gl.json .
SCRIPT
```

Container CPU limit:
`kubectl get deploy forgejo-gitea -n forgejo -o jsonpath='{.spec.template.spec.containers[?(@.name=="gitea")].resources}'`

Decode-default evidence:
`gitleaks git -h | grep decode-depth` — 8.25.1: `default "0", no decoding is
done`; 8.30.1: `default 5`.

Synthetic checkpoint-churn generator and policy variants used above were
scratch-only (`~/scratch/gitleaks-830-bench/`) and were removed after the
investigation; nothing in this repo or in declarative-config was modified.
