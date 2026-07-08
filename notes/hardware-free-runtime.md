# Hardware-Free Runtime Reproduce

**Canonical script:** `scripts/run-sim-local.sh`
**Verified:** 2026-07-08 — `5bbe52c` (post bf-243os sim→blob path fix), re-verified at HEAD `f859f3e` for bf-40hc acceptance, and final-clean-rebuild re-verified at HEAD `c927f67` (the bf-40hc doc commit itself).
**Resolves:** the ambiguity that caused bf-40hc's repeated failures; closes the chain bf-3zll → bf-3hji → bf-51fq → bf-5cr3, and **satisfies bf-40hc** (hardware-free runtime path that creates a blob).

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

## Acceptance — bf-40hc (independently re-verified 2026-07-08, HEAD `f859f3e`)

`./scripts/run-sim-local.sh` is the bf-40hc deliverable. Re-running it at `f859f3e`
produced `exit 0` with this evidence, mapping each acceptance criterion:

| bf-40hc acceptance criterion | Verified evidence (2026-07-08, HEAD `f859f3e`) |
|------------------------------|-----------------------------------------------|
| Documented command starts the mothership without a runtime panic/crash | `[run-sim] mothership healthy` after the `/healthz` poll; clean startup, no panic — mothership served the full 25 s run and shut down via the trap |
| At least one blob object observable at runtime (API / WebSocket / tracker log) | Independent `/api/blobs` poller: `peak_blob_count=2`, `first_blob_at=1s` (nodes_online=4 throughout); `sim --verify` secondary cross-check: `[SIM] PASS: 2 blobs detected for 2 walkers` |
| Reproduce command written into the bead body or a referenced note | This file + `scripts/run-sim-local.sh`; exact flags: `--nodes 4 --walkers 2 --rate 30 --space 5x5x2.5 --duration 25 --seed 42 --verify` |

Reference run captured: `peak=2  first_blob=1s  sim_exit=0  verify=PASS  rejects=0  fps≈363`.

**Out of scope — do not confuse with bf-40hc:** the `tests/e2e` tests
`TestDetectionEvents`, `TestFullE2EIntegration`, and `TestConcurrentNodes` are
**deliberately RED by design** (see their inline NOTE comments) — they are gated on
the upstream fusion `SetNodePosition` wiring (**bf-4q5w**) and are the strictness
contract of the **bf-5jeo** verification capstone. `TestConcurrentNodes` additionally
connects raw tokenless nodes against the harness's `SPAXEL_MIGRATION_WINDOW_HOURS=0`
strict mode, so its rejects are a harness artifact, not a pipeline defect. They must
NOT be weakened to turn green, and they are not a bf-40hc regression: bf-40hc asserts
only that *a* hardware-free runtime path emits a tracked blob, which `run-sim-local.sh`
deterministically proves.

---

## BLE identity matcher (--ble) — bf-1yr1w

`run-sim-local.sh` proves the CSI→blob path. **`scripts/run-sim-ble-match.sh`** layers
the `--ble` opt-in on top and asserts the identity **matcher** (`ble.IdentityMatcher`)
resolves a seeded person onto a *live CSI blob* — the second link of the bf-2m534 identity
chain. The first link (register a person + bind the sim's advertised device) is the
**fixture**, documented in **[`notes/ble-identity-fixture.md`](ble-identity-fixture.md)** —
link to it, do not duplicate it. The matcher script reuses that exact REST sequence
internally.

### `--ble` opt-in in `run-sim-local.sh` (`SIM_BLE=1`)

The canonical script now carries the `--ble` opt-in itself (bf-1yr1w), so you don't
have to fork it to exercise BLE ingestion:

```bash
SIM_BLE=1 ./scripts/run-sim-local.sh      # appends --ble to the sim invocation
./scripts/run-sim-local.sh                # default: --ble OFF, canonical CSI→blob path unchanged
```

Default-off — `run-sim-local.sh`'s gate is still the CSI→blob path. Re-verified green
both ways at HEAD `c42989e` (2026-07-08): default `peak=2` and `SIM_BLE=1` `peak=2`,
both with `[SIM] PASS: 2 blobs detected for 2 walkers` / `sim --verify PASSED`. The
opt-in only turns on BLE advertisement ingestion; it does **not** seed a person or
assert identity. The fixture + matcher gate is `scripts/run-sim-ble-match.sh` below.

### The recipe

```bash
./scripts/run-sim-ble-match.sh
# builds mothership + canonical sim (mothership/cmd/sim — has --ble; never repo-root cmd/sim),
# starts a fresh-data-dir mothership, applies the fixture (register person "Alice" +
# bind sim walker-0 addr AA:BB:CC:DD:EE:00), then runs:
#   sim --ble --nodes 4 --walkers 1 --rate 30 --space 5x5x2.5 --duration 25 --seed 42
# polls GET /api/ble/matches and exits 0 once GetMatch() returns the seeded person
# attached to a REAL blob (blob_id != -1).
```

### Params (why these, not run-sim-local's)

| Env | Value | Note |
|-----|-------|------|
| `SIM_WALKERS` | **`1`** | NOT the `2` from `run-sim-local.sh` — see gotcha below |
| `SIM_RATE` | `30` | same as gotcha #2; deterministic CSI blobs the matcher needs |
| `SIM_DURATION` | `25` | same as run-sim-local |
| `SIM_SEED` | `42` | pins walker paths; walker 0 always advertises `AA:BB:CC:DD:EE:00` |
| `SIM_NODES` | `4` | multi-node RSSI → triangulation clears MinMatchConfidence |
| `SIM_SPACE` | `5x5x2.5` | default room geometry |
| `SIM_PORT` | `8088` | loopback; avoids 8080 |
| `SIM_WALKER_MAC` | `AA:BB:CC:DD:EE:00` | sim walker-0 convention (see fixture note) |
| `SIM_PERSON_NAME` / `SIM_PERSON_COLOR` | `Alice` / `#4488ff` | the seeded identity |

### Gotcha — `walkers=1`, not `2`

The fixture registers **exactly one** person + device, so the sim must emit exactly one
walker (Alice's). With `walkers=2` (run-sim-local's blob-stress default) the CSI blob
nearest Alice's triangulated point was the **second** walker's (cornered away from her),
leaving a ~1.73 m gap so `f_distance` killed `matchConf` (0.143 < gate 0.60) — diagnosis
bf-6crd7 / bf-6d2ii. With one walker the blob is Alice's own, co-located at ~0.6 m →
`matchConf ≈ 0.71`. This is **not** a threshold loosening: `MaxBLEBlobDistance=2.0` and
`MinMatchConfidence=0.6` are unchanged.

### Observation surface

`GET /api/ble/matches` → `identityMatcher.GetAllMatches()` (`cmd/mothership/main.go:~3070`).
It returns **both** real blob matches (`blob_id >= 0`) and BLE-only placeholder tracks
(`blob_id == -1`: triangulated device with no nearby blob). A PASS record is a real blob
match — `blob_id != -1` AND `person_name == "Alice"` AND `confidence >= 0.6`:

```json
{"blob_id":7,"person_name":"Alice","device_addr":"AA:BB:CC:DD:EE:00",
 "confidence":0.714,"is_ble_only":false}
```

### PASS evidence (HEAD `7757149`; re-verified `c42989e`, 2026-07-08)

```
[match] peak_blobs (via /api/blobs): 2
[match] alice_blob_match: blob_id=7 conf=0.7139527093753523 device=AA:BB:CC:DD:EE:00 (at 6s)
[match] final /api/ble/matches summary:
  [{"blob_id":7,"person_name":"Alice","device_addr":"AA:BB:CC:DD:EE:00",
    "confidence":0.7139527093753523,"is_ble_only":false}]
[match] PASS: GetMatch() returns "Alice" for blob_id=7 (conf=0.714) via /api/ble/matches
```

Closes bf-1yr1w criterion 3: the `--ble` matcher recipe is now recorded here, not only in
the fixture note. The matcher → `/api/blobs` identity capstone is **bf-2m534**
(`scripts/run-sim-identity.sh`), which depends on this PASS — keep it green.

### Identity capstone PASS — bf-gdfwx (live `/api/blobs`, 2026-07-08)

`scripts/run-sim-identity.sh` is the THIRD and final link of the identity chain
(matcher-active → match-found → written-onto-served-blob → observable-at-`/api/blobs`).
It drives the same hardware-free runtime with `--ble`, seeds the BLE registry with a
registered person BEFORE the sim starts (the fix for bf-gdfwx's two prior failures —
the early CSI blob window is narrow at walkers=1), then asserts a live `/api/blobs` blob
carries non-empty canonical identity.

Run (reproduce): `./scripts/run-sim-identity.sh` — exit 0. Flags/sequence:
`nodes=4 walkers=1 rate=30 duration=25s seed=42 ble=on SIM_PERSON_NAME=Alice
SIM_WALKER_MAC=AA:BB:CC:DD:EE:00 SIM_PORT=8088`, fixture applied pre-sim, 0.5s poll cadence.

Live `/api/blobs` blob (the served, canonical-identity-bearing blob):
```json
{"ID":7,"X":2.5,"Y":2.3,"Z":1.9,"Weight":0.6923,
 "person_id":"6960f453-c765-4018-a180-9fbeddfc1b3a","person_label":"Alice",
 "person_color":"#ec4899","identity_confidence":0.6000597064722052,"identity_source":"ble",
 "personName":"Alice","assignedColor":"#ec4899","identityResolved":true}
```
Corroboration from the runtime matcher surface (`GET /api/ble/matches`):
```json
[{"blob_id":7,"person_name":"Alice","device_addr":"AA:BB:CC:DD:EE:00",
  "confidence":0.6000597064722052,"is_ble_only":false}]
```
Satisfies bf-gdfwx acceptance: canonical identity populated **and** observed from the
live REST response (`personName="Alice"`, `assignedColor="#ec4899"`,
`identityResolved=true`), no panic, evidence recorded here (reused, not re-derived).

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
