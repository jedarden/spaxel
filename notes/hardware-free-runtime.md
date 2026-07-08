# Hardware-Free Runtime Reproduce

**Canonical script:** `scripts/run-sim-local.sh`
**Verified:** 2026-07-08 at HEAD `5bbe52c` (post bf-243os sim→blob path fix)
**Resolves:** the ambiguity that caused bf-40hc's repeated failures; closes the chain bf-3zll → bf-3hji → bf-51fq → (this) bf-5cr3.

This is the single deterministic artifact that captures the working end-to-end
runtime path — **sim connects → CSI ingested → phase/fusion → tracked blob** — with
zero hardware, zero Docker, zero manual IP, and zero manual token. Later beads
(e.g. the blob-identity runtime checks under **bf-f841**) and reruns should start
from here instead of re-deriving the command sequence or the tuning.

---

## TL;DR

```bash
./scripts/run-sim-local.sh
# expects, within ~30s:
#   [run-sim] mothership healthy
#     t= 1s  nodes_online=4  blobs=2  peak=2
#     ...
#   [run-sim] peak_blob_count=2
#   [run-sim] PASS: >=1 blob via /api/blobs (peak=2) AND sim --verify passed
```

Exit 0 iff ≥1 tracked blob is observed via `GET /api/blobs` over the run window.

---

## What the script does (the exact reproduce sequence)

1. **Build mothership from source** — `cd mothership && go build -o /tmp/spaxel-mothership ./cmd/mothership`
2. **Build the canonical sim from source** — `cd mothership && go build -o /tmp/spaxel-sim ./cmd/sim`
3. **Start the mothership** on an ephemeral port + data dir:
   ```bash
   SPAXEL_BIND_ADDR=127.0.0.1:8088 \
   SPAXEL_DATA_DIR=$(mktemp -d) \
   SPAXEL_LOG_LEVEL=warn \
   SPAXEL_MDNS_ENABLED=false \
   TZ=UTC \
     /tmp/spaxel-mothership &
   ```
4. **Wait for health** — poll `GET /healthz` until `{"status":"ok"}`.
5. **Run the sim** with `--verify`:
   ```bash
   /tmp/spaxel-sim \
     --mothership ws://localhost:8088/ws/node \
     --nodes 4 --walkers 2 --rate 30 --space 5x5x2.5 \
     --duration 25 --seed 42 --verify
   ```
6. **Observe** — poll `GET /api/blobs` once per second for the whole streaming
   window, keep the peak, gate on `peak ≥ 1`. (`sim --verify` is a reported
   secondary cross-check that asserts `blob_count == walkers ±1`.)

Each step is in `scripts/run-sim-local.sh` with a `trap` cleanup that kills both
processes and removes the temp data dir.

---

## The two gotchas (read these before touching the path)

### 1. There are TWO `cmd/sim` packages — only one has `--verify`

| Path | Module | Has `--verify`? | Use it? |
|------|--------|-----------------|---------|
| `mothership/cmd/sim/` | part of the mothership module | **YES** (`verify.go`) | **YES — canonical** |
| `cmd/sim/` | separate, older module (own `go.mod`) | no | **NO** |

`scripts/run-sim-local.sh` builds **only** from `mothership/cmd/sim` and
additionally guards against a stale binary by checking `--help` for `--verify`,
rebuilding if missing. **Do not** build/run `cmd/sim` for runtime blob checks.

### 2. `rate=30`, not the upstream default `20`

At the upstream default `--rate 20`, fusion produces a peak only *while* a walker
moves enough to cross the DeltaRMS threshold, so a run can legitimately land
`peak=2` *or* `peak=0` nondeterministically (see the comment in
`mothership/cmd/sim/verify.go`). Bumping `--rate` to **30** reliably clears the
threshold: every observed run hits `peak == walkers` with the first blob at ~1s.
This is a rate/tuning fact, **not** a broken pipeline — if a bead reports 0 blobs,
first re-run at `rate=30` before suspecting a defect.

`--duration 25` (not 40) keeps the sim's late-window `--verify` sample inside the
active-walk phase; at long durations walkers drift into low-signal positions and
verify can spuriously report 0 even though the `/api/blobs` window still saw blobs.

---

## Token path (no manual token needed)

The canonical sim authenticates exactly like real provisioned firmware:

- `--token` defaults to empty → the sim calls `POST /api/provision` to mint a real
  per-node HMAC token (`HMAC-SHA256(install_secret, node_mac)`), sent as the
  `X-Spaxel-Token` header on the WebSocket dial, then bridged into `hello.Token`
  for validation (bf-1o7qi).
- Fallback: if provisioning fails, the sim generates a synthetic token. Either
  way the connection is not left tokenless-by-design.
- Defense-in-depth: the mothership also tolerates tokenless nodes for
  `SPAXEL_MIGRATION_WINDOW_HOURS` (default 24h) after startup, so even a token
  glitch during a reproduce run does not cause a `reject` (bf-2hdbg). The script
  counts `reject`/`401`/`403` lines in the sim log and reports the count; expect 0.

If you ever see rejects, check `SPAXEL_MIGRATION_WINDOW_HOURS` (the e2e harness
sets it to `0` to force strict validation) and that the binary is built from
`mothership/cmd/sim`.

---

## Environment (mothership)

| Variable | Value in the script | Why |
|----------|---------------------|-----|
| `SPAXEL_BIND_ADDR` | `127.0.0.1:8088` | Loopback; avoid clashing with anything on 8080 |
| `SPAXEL_DATA_DIR` | ephemeral `mktemp -d` | Clean state every run; removed on exit |
| `SPAXEL_LOG_LEVEL` | `warn` | Quiet enough to scan, loud enough on real problems |
| `SPAXEL_MDNS_ENABLED` | `false` | Loopback reproduce needs no multicast; sim dials directly |
| `TZ` | `UTC` | Deterministic diurnal/timestamp behavior |

## Parameters (sim)

| Flag | Default in script | Notes |
|------|-------------------|-------|
| `--nodes` | `4` | Enough for stable multi-link fusion |
| `--walkers` | `2` | `peak == walkers` at rate=30 |
| `--rate` | `30` | **See gotcha #2** — 20 is nondeterministic |
| `--duration` | `25` | Keeps `--verify`'s late sample in the active-walk phase |
| `--seed` | `42` | Reproducible walker paths |
| `--space` | `5x5x2.5` | Default room geometry |
| `--verify` | (set) | Canonical acceptance: `blob_count == walkers ±1` + within-2m check |

All overridable via env: `SIM_NODES`, `SIM_WALKERS`, `SIM_RATE`, `SIM_DURATION`,
`SIM_SEED`, `SIM_SPACE`, `SIM_PORT`. Binary paths via `SPAXEL_MOTHERSHIP_BIN` /
`SPAXEL_SIM_BIN`.

---

## Expected result (reference run, HEAD `5bbe52c`)

```
peak_blob_count=2
first_blob_at=1s
samples=25  nodes=4 walkers=2 rate=30 duration=25s seed=42
sim_exit=0  (0 = --verify PASSED)
sim_verify: [SIM] PASS: 2 blobs detected for 2 walkers
reject_in_sim_log=0
fps=Average FPS: ~360
```

---

## For later beads (bf-f841 and beyond)

- **Reuse, don't re-derive.** Start from `scripts/run-sim-local.sh`. If you need a
  runtime assertion, run the script (or import its param block) rather than
  hand-rolling a `curl /api/blobs`.
- **Determinism contract:** `--seed` pins walker paths; `--rate 30` pins blob
  emission. The only residual nondeterminism is OS scheduling jitter, which the
  peak-over-window poller absorbs.
- **Adding identity checks (bf-f841):** the sim emits synthetic BLE advertisements
  with `--ble`; pair `--ble` with the BLE-registry REST flow. The token/CSI/blob
  path this script exercises is the prerequisite — keep it green.
- **Predecessor:** `blob_observation.sh` (repo root) is the bf-51fq-specific
  harness with the same logic and detailed inline comments; this script is the
  bead-agnostic canonical form that supersedes it for general use.

## Keeping this honest

If a clean run of `scripts/run-sim-local.sh` ever fails to reach `peak ≥ 1`:
1. Confirm HEAD includes the sim→blob fix (the `bf-243os` commit and
   `b9f362c` LinkID-split fix) — `grep` `fusion.go` for stray debug flags.
2. Rebuild both binaries (`rm /tmp/spaxel-sim /tmp/spaxel-mothership` and rerun).
3. Try `SIM_RATE=35` to rule out a jitter-induced threshold miss.
4. Only then treat it as a real defect and update this note + the script defaults.
