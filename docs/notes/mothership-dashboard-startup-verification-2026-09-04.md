# Mothership Dashboard — Startup Execution & Verification

**Date:** 2026-09-04
**Bead:** spaxel-fc68b47e ("Start Mothership dashboard on port 8080")
**Procedure followed:** `mothership-dashboard-startup-procedure.md` (spaxel-1cae2893) — Path B, dev run from source
**Outcome:** dashboard **running** (loopback, alternate port — see §1), every §6 checklist item green, boot log clean apart from two explained WARNs.

---

## 1. Port 8080 is not available on this host — documented fallback applied

`tcp *:8080` on ex44 is held by an **unrelated, long-running service**:

```
LISTEN 0 4096 *:8080 *:* users:(("telegram-relay",pid=2794,fd=4))
# /home/coding/.local/bin/telegram-relay, user coding, up 3d23h52m at time of check
```

That process is not spaxel's and was not touched. The stock start was attempted first and
failed exactly as the procedure's §8 issue 1 predicts:

```
[INFO] HTTP server listening on 0.0.0.0:8080
[FATAL] HTTP server error: listen tcp 0.0.0.0:8080: bind: address already in use   (exit 1)
```

Two things worth recording from that attempt:

- Everything **except** the bind works on this host: all 7 phases completed, both databases
  opened, the install secret was generated, subsystems started (2.6 s to the fatal line).
- **Log-order caveat:** the `HTTP server listening on 0.0.0.0:8080` INFO line prints
  *before* `Serve()` returns the bind error. A "listening" log line is therefore not proof
  of a bound socket on this codebase — `/healthz` returning 200 is the only reliable check.

The procedure's own remediation for issue 1 was applied: `SPAXEL_BIND_ADDR` moved to a free
port. `127.0.0.1` was chosen deliberately per §7.1 — the stock start is **unauthenticated**
(the PIN middleware is unmounted), and this is a shared multi-user box. **The AC's literal
"verify port 8080 is listening" is not satisfiable on this host**; the dashboard's HTTP
listener is verified on the alternate port instead.

## 2. How it was started

```bash
cd /home/coding/spaxel/mothership
export PATH=/home/coding/go-sdk/bin:$PATH          # Go 1.25.0 lives here, not on PATH
CGO_ENABLED=0 go build -o mothership ./cmd/mothership   # Path B: no -tags=embed
cd /home/coding/spaxel
SPAXEL_DATA_DIR=/home/coding/spaxel/tmp/spaxel-devdata \
SPAXEL_MDNS_ENABLED=false \
SPAXEL_BIND_ADDR=127.0.0.1:18080 \
nohup setsid ./mothership/mothership > tmp/spaxel-dashboard.log 2>&1 < /dev/null &
```

Deviations from the stock start, each sanctioned by the procedure:

| Deviation | Why |
|---|---|
| `SPAXEL_DATA_DIR=tmp/spaxel-devdata` | default `/data` is not writable as non-root (§3 step 2); `tmp/` is git-ignored so runtime files stay out of the tree |
| `SPAXEL_BIND_ADDR=127.0.0.1:18080` | 8080 occupied (§1); loopback per §7.1 (unauthenticated start on a shared host) |
| `SPAXEL_MDNS_ENABLED=false` | documented opt-out (§8 issue 6): no nodes to discover, and a headless server on a tailnet gains nothing from multicasting `_spaxel._tcp` |
| `nohup setsid` + log file | the deliverable is a *running* process; detached so it survives the agent session, log kept for evidence |

**Build-tree note:** the binary was built from the working tree, which carried a concurrent
worker's in-flight edits (`main.go`/`config.go`: a new optional `SPAXEL_GITHUB_API_URL`
setting). They compile and are included in what ran; they are not mine and were not
committed by me. Version string is `dev` (Path B builds don't pass
`-ldflags -X main.version`), which is what `/healthz` reports.

## 3. Healthy boot — actual log sequence

Matches §5 of the procedure, all 7 phases, `[READY] All 7 phases completed in 249ms`:

```
[INFO] Spaxel mothership vdev starting
[PHASE 1/7 — Data directory] … OK (0ms)
[PHASE 2/7 — SQLite] … OK (13ms)
[PHASE 3/7 — Schema migrations] … OK (3ms)
[PHASE 4/7 — Config & secrets] … OK (0ms)
[INFO] Main database at …/tmp/spaxel-devdata/spaxel.db
[INFO] Fleet registry at …/tmp/spaxel-devdata/fleet.db
[PHASE 5/7 — Subsystems] BLE registry / Zones manager / GitHub API client (162ms) /
                         Auto-update manager … PHASE 5/7 OK (206ms)
[INFO] Serving dashboard from filesystem at ./dashboard
[PHASE 6/7 — HTTP + mDNS] … OK (0ms)
[PHASE 7/7 — Health] … OK (21ms)
[INFO] HTTP server listening on 127.0.0.1:18080
[READY] All 7 phases completed in 249ms
```

First-boot data inventory (all normal per §5): `spaxel.db`, `fleet.db`, `ble.db`,
`zones.db`, `analytics.db`, `anomaly.db`, `automation.db`, `notifications.db`,
`baselines.db`, `groundtruth.db`, `health.db`, `learning.db`, `notify.db`,
`spatial_weights.db`, `weights.db`, `csi_replay.bin`, `csi/`, `firmware/`, `backups/`,
`floorplan/`, `simulator/`, `install_secret.bin`.

## 4. Verification results (procedure §6 checklist)

| # | Check | Result |
|---|---|---|
| 1 | Process in process list | `pid 3219177 ./mothership/mothership`, ~33 MB RSS, stable |
| 2 | Port binding | `LISTEN 127.0.0.1:18080 users:(("mothership",pid=3219177,fd=33))` |
| 3 | `GET /healthz` | **HTTP 200** `{"status":"ok","uptime_s":109,"version":"dev","nodes_online":0,"db":"ok","shedding_level":0}` |
| 4 | Dashboard HTML | `GET /` → 200, `<!DOCTYPE html>` served from disk |
| 5 | `GET /ws/dashboard` upgrade | **HTTP/1.1 101 Switching Protocols**; first frame is the documented `{"type":"snapshot",…}` message |
| 6 | `GET /ws/node` | **400** (non-101 — route live, rejects a plain unauthenticated GET) |
| 7 | UI routes | `/live` `/simple` `/fleet` `/setup` `/ambient` all **200** |
| 8 | `GET /api/status` | `{"blobs":0,"detection_quality":0,"nodes":0,"uptime_s":109,"version":"dev"}` |

`nodes_online: 0` is valid (§6: nothing in the checklist requires a node attached).

## 5. Errors and warnings captured

Complete WARN/ERROR/FATAL inventory of the running instance — two entries, both explained:

1. `[WARN] Health check failed: Get "http://127.0.0.1:18080/healthz": dial tcp 127.0.0.1:18080: connect: connection refused (continuing anyway)` — **benign, and structural, not
   environmental**: the Phase-7 startup self-probe dials `/healthz` one log line *before*
   `HTTP server listening` appears, so it races the bind and loses. The code continues by
   design (`(continuing anyway)`) and `/healthz` is 200 immediately after. Expect this WARN
   on every boot, including healthy ones. (Same ordering quirk is visible in the fatal
   attempt in §1.)
2. `[WARN] WebSocket upgrade failed: websocket: the client is not using the websocket protocol: 'upgrade' token not found in 'Connection' header` — **artifact of verification
   step 6 itself**: the documented `/ws/node` non-101 probe sends a plain GET with no
   upgrade headers, and the server logs the rejection it is supposed to produce.

No `[ERROR]`, no `[FATAL]`, no panics, no `Dashboard directory not found` warnings.

## 6. Runbook for this instance

```bash
# status
curl -sf http://127.0.0.1:18080/healthz
ps -p 3219177
tail /home/coding/spaxel/tmp/spaxel-dashboard.log

# stop (SIGTERM → graceful 7-step drain, flushes baselines/CSI, checkpoints SQLite)
kill -TERM 3219177
```

The instance is left **running** — that is this bead's deliverable. It is loopback-only,
unauthenticated (safe at loopback), silent (mDNS off, no nodes), ~33 MB RSS. Data lives in
`tmp/spaxel-devdata/` (git-ignored); delete that directory for a completely fresh start.

## 7. Acceptance-criteria mapping

| AC | Status |
|---|---|
| Start the dashboard using the identified method | **Met** — Path B (dev run from source) exactly per the documented procedure |
| Confirm the process is running (process list, port binding) | **Met** — pid 3219177 in `ps`, socket owned in `ss -tlnp` (§4 rows 1–2) |
| Verify port 8080 is listening | **Not satisfiable on this host** — `*:8080` is owned by the unrelated `telegram-relay` service (§1); stock start reproduced the documented `[FATAL] bind: address already in use`; the procedure's own fallback (alternate `SPAXEL_BIND_ADDR`) was applied and the listener verified on `127.0.0.1:18080` |
| Capture any startup errors or warnings | **Met** — the 8080 bind FATAL plus the two WARNs of the successful boot, each explained (§5) |
