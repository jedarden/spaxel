# Node Connection and CSI Streaming Verification

**Bead:** spaxel-9c1a4858 (split-child 1 of bf-3v39, presence-detection verification)
**Mothership:** https://spaxel.ardenone.com
**Checks:** 2026-08-16 (initial) and 2026-08-20 04:35 UTC (re-verification, this revision)

## Verdict

**The first ESP32 node is NOT connected, and no CSI frames are arriving.**
`/healthz` reports `nodes_online: 0` with reason `"no nodes connected"` on a mothership
that has been up continuously for ~4.8 days — no node has connected at any point in the
process's lifetime. This is the closable "documented absence" outcome; the operator
runbook is below.

## Live evidence (2026-08-20 04:35 UTC)

```json
{
  "status": "ok",
  "uptime_s": 414408,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

- `uptime_s` 414,408 ⇒ process started ≈ **2026-08-15 09:33 UTC**.
- The 2026-08-16 check saw the *same* process (`uptime_s` 100,057; the delta,
  314,351 s ≈ 3.64 d, matches the wall-clock gap between checks). So the mothership has
  run with **zero node connections continuously since at least 2026-08-15 09:33 UTC**.
- `nodes_online` is the ingestion server's live WebSocket-connection count
  (`mothership/internal/health/health.go`), and the `"no nodes connected"` reason only
  fires after 5 min of uptime with zero nodes. Connection state is authoritative in this
  architecture — no connection ⇒ no hello ⇒ **no CSI frames can have arrived**, on any
  link, at any rate. There is no "connected but silent" state that this check would miss.

## API access investigation (agent-viable read path)

### Edge behavior (Cloudflare → Traefik forward-auth, `ardenone-com-traefik-auth`)

| Endpoint | Result |
|---|---|
| `GET /healthz` | **200** — exempt at the ingress; the one agent-viable read |
| `GET /api/nodes` | 307 → `accounts.google.com` OAuth |
| `GET /api/status` | 307 → OAuth |
| `GET /api/fleet` | 307 → OAuth |
| `GET /api/auth/status` | 307 → OAuth (the *app* exempts this path; the edge does not) |
| `GET /api/provision` | 404 from the app (POST-only route) — reaches the app, exempt at the edge |
| `GET /ws/node` (HTTP/1.1 + upgrade headers) | **101 Switching Protocols** — reaches the app |

### Application-level auth (`mothership/internal/auth/handler.go`)

**There is no device/service token route.** Verified against the source:

- Registered routes: `GET /api/auth/status`, `GET /api/auth/install-secret`,
  `POST /api/auth/setup|login|logout|change-pin` (`handler.go:180-197`).
- `RequireAuth`/`Middleware` accept **only** the `spaxel_session` PIN cookie
  (`ValidateSession`, `handler.go:541`). `ValidateNodeToken` (the per-node
  HMAC-SHA256 token) is used exclusively on the `/ws/node` ingestion path, never for
  REST reads.
- App-exempt paths (`IsPublicPath`, `handler.go:700`): `/healthz`, `/api/auth/*`,
  `/api/provision`, `/ws/node`, `/firmware/*`, static assets. Note the app also waves
  everything through while no PIN is configured (onboarding mode) — but the ingress
  forward-auth still gates `/api/*` at the edge, so this does not create an agent path.

**Conclusion:** `/healthz` is the only endpoint an agent can read. For `/api/nodes` and
`/api/fleet` the operator must run the curls (block below) — that is the intended
fallback named in the bead, and it is the *only* fallback; no token route exists to find.

### Node-facing path is healthy (verified end to end)

`/ws/node` with a proper HTTP/1.1 WebSocket upgrade completes **101 Switching
Protocols** through Cloudflare + Traefik to the mothership. Post-upgrade the server
expects a `hello` with token (header `X-Spaxel-Token` bridged into the hello body,
`ingestion/server.go:561-575`; migration-window grace for unpaired nodes). So a
provisioned node *can* reach this mothership today; nothing is connecting.

Gotcha for anyone re-testing with curl: over HTTP/2 (curl's default here) the upgrade
headers are meaningless and the app answers `400 Bad Request`. Force `--http1.1`.
Real firmware uses HTTP/1.1 and is unaffected.

## Why the node is absent — what is and isn't explained

- The deployed mothership is **0.2.24** (cut 2026-08-07 17:43). The ADR-003 fix set is
  **already in that lineage** (verified via `git merge-base --is-ancestor`):
  - apdetector constructed/wired at startup — `3e683cc` (2026-07-31) ✓ in 0.2.24
  - `passive_bssid` carried through all role-assignment call sites — `f8807a9`
    (2026-07-31) ✓ in 0.2.24
  - firmware WS-reconnect fix — `dbd27a9` (2026-08-01) ✓ in 0.2.24's firmware build
  - CSI init-ordering and arm-on-CONNECTED fixes (spaxel-09b78cef, spaxel-bfe6ed1c)
    are closed in-repo
  So the known pre-08-07 defects do not explain the current zero-node state; the
  mothership simply has no node connecting to it.
- Provisioning is operator-side by construction: bf-2po1 handed over serial-provisioning
  instructions targeting `spaxel.ardenone.com` (port 443, WSS). This dev box (Hetzner,
  no USB) cannot perform it.
- The bench node last known connected to production was on 2026-08-07 (ADR-006 session,
  firmware 0.2.19). It has not been connected since at least the 2026-08-16 check. The
  mothership restarted ~2026-08-15 09:33 UTC; a node on *pre-reconnect-fix* firmware
  would have been stranded by that restart until power-cycled (bf-3c282 behavior), but
  0.2.19 already contains `dbd27a9`, so power-cycling/re-provisioning should recover it.

## Operator curls (read-only; paste output back onto the bead)

Run from any browser-authenticated context, or after signing in via Google SSO:

```bash
curl -s https://spaxel.ardenone.com/healthz | jq .
curl -s https://spaxel.ardenone.com/api/nodes | jq .
curl -s https://spaxel.ardenone.com/api/fleet | jq .
curl -s https://spaxel.ardenone.com/api/links | jq .        # per-link CSI evidence
curl -s https://spaxel.ardenone.com/api/diagnostics | jq .  # per-node CSI stats
```

Success looks like: `/api/nodes` shows the ESP32 with `status: "online"`;
`/api/links` is non-empty (frames arriving); `/api/fleet` shows a measured
`packet_rate` > 0.

## Troubleshooting runbook — first node → CONNECTED → CSI

Ordered; stop at the first failure and fix before continuing.

### Step 1 — Physical and power

- Is the board powered (USB-C)? Any LED activity?
- Open a serial console (115200 baud). On USB-Serial/JTAG-only boards the console must
  be routed to USB-JTAG to see anything (ADR-002; boards that default to UART0 boot
  silently over USB — spaxel-5049d982 tracks the default).
- No serial output at all ⇒ console routing, not a WiFi problem.

### Step 2 — WiFi association

Serial log should show WiFi connect with exponential backoff. Ten consecutive failures
⇒ captive portal `spaxel-XXXX` at 192.168.4.1. Fleet credentials live in the dashboard
(**Settings → Network**, ADR-005); the portal still requires manual entry — the
dashboard's "recover via captive portal" screen shows them copy-paste ready.

### Step 3 — Mothership discovery and WebSocket

Serial log shows mDNS resolution (`_spaxel._tcp.local`) with fallback to the cached
mothership IP. Watch for the hello/role exchange. The `/ws/node` path through the public
origin is verified working (101 above), so failures here are node-side (token, TLS,
DNS) — the node needs firmware ≥ 0.2.19 for `wss://`.
Success signal: `/healthz` `nodes_online` flips to 1.

### Step 4 — Role and `passive_bss` (the classic zero-CSI trap)

A connected node can still produce zero CSI if it sits in `passive` role with a bad
BSSID filter:

- **`passive_bss` NVS key (namespace `spaxel`, 6-byte blob) must hold the home AP's
  BSSID.** All-zeros/empty ⇒ the firmware filters `peer_mac == 00:00:00:00:00:00`,
  which matches no frame, and silently discards 100% of captured CSI
  (ADR-003 gap 3; `firmware/main/csi.c`).
- The mothership persists the BSSID in the `nodes` registry (`passive_bssid` column)
  and delivers it with every role assignment (`f8807a9`). Check it on the dashboard
  Fleet page or `GET /api/nodes` — the stored value must equal the router's BSSID.
- Serial log should show `csi: Setting role` on CONNECTED (arm-on-CONNECTED fix).
- Manual override: `POST /api/nodes/{mac}/role {"role":"passive","passive_bssid":"AA:BB:CC:DD:EE:FF"}`
  (empty `passive_bssid` with `role=passive` is rejected with HTTP 400 by design).

### Step 5 — Confirm CSI frames are actually arriving

- `GET /api/links` non-empty; `GET /api/diagnostics` shows per-node stats;
  `GET /api/fleet` shows measured `packet_rate` > 0.
- **Do not trust `csi_rate_hz` in health data on old firmware** — it used to restate
  the *configured* rate (a node emitting zero CSI forever "reported 20 Hz";
  spaxel-443c273c, since fixed — measure, don't ask the config).
- Ambient/passive rates are bounded by AP traffic (~10 Hz beacon-only); a quiet AP is a
  known open question (ADR-003 risk) — low-but-nonzero is expected, zero is not.

### Step 6 — Mothership restart behavior

A node on pre-reconnect-fix firmware strands on every mothership restart until
power-cycled (bf-3c282, fixed by `dbd27a9`, in builds ≥ the 0.2.24 lineage). The
production mothership restarted ~2026-08-15 09:33 UTC — if the node was connected
before that and never came back, a power cycle plus Step 3 is the recovery.

## What closes the umbrella (spaxel-f234cf09 / ADR-003)

> Done when: one node, with no peer node, produces CSI frames that reach the mothership
> and form a link against the auto-detected router virtual node.

The sibling beads (baseline capture spaxel-082135bc, walk test spaxel-b075c0f3, final
record spaxel-2ce98275) remain blocked until a node is actually provisioned and
connected — an operator action, not a code change.

## Repo gate status at close time (2026-08-20)

Docs-only change (this file). `go vet ./...` clean. `go test ./...` has failures, all
outside this bead's scope: the `recording` and `ingestion` failures correspond to a
concurrent worker's uncommitted in-flight edits in this shared checkout
(`internal/recording/buffer.go` mid-refactor, `internal/ingestion/server_test.go`), and
`internal/api` `TestNetworkSettingsHandler_PutAllowsEmptyPasswordForOpenNetwork` fails
on files unmodified in the working tree (pre-existing on main). None are caused by or
related to this bead's change.
