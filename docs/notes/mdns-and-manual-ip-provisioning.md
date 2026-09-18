# Node Provisioning: mDNS Discovery and Manual-IP Fallback

How a freshly-flashed node finds the mothership, what the operator does for
each mode, and what to check when discovery fails.

Covers both supported modes and the exact provisioning workflow for each:

- **Mode 1 — mDNS auto-discovery** (default): the mothership advertises
  `_spaxel._tcp.local.` on the LAN; nodes resolve it after joining WiFi.
- **Mode 2 — manual IP fallback** (mDNS-less networks): the provisioning
  payload carries the mothership's IP directly and mDNS is skipped.

Implementation references: mothership advertisement
(`mothership/cmd/mothership/main.go`, `mdns_binding.go`), provisioning payload
(`mothership/internal/provisioning/server.go`), node-side resolution
(`firmware/main/main.c` `NODE_STATE_MOTHERSHIP_DISCOVERY`,
`firmware/main/wifi.c` `wifi_discover_mothership`), wizard
(`dashboard/js/onboard.js`). Integration coverage:
`mothership/tests/e2e/mdns_discovery_test.go` (bead spaxel-503359c5).

## The provisioning payload

`POST /api/provision` returns the blob the Web Serial wizard writes to the
node's NVS. The reachability fields:

| Field     | Meaning                                                            |
|-----------|--------------------------------------------------------------------|
| `ms_mdns` | mDNS instance name to resolve (`SPAXEL_MDNS_NAME`, default `spaxel`) |
| `ms_port` | Port the mothership's HTTP/WS server actually listens on (derived from `SPAXEL_BIND_ADDR`) |
| `ms_ip`   | Optional manual override; present only when the wizard sent one    |

`ms_port` is always the real listener port — the advertisement's SRV record
carries the same value, so a node that provisions and a node that discovers
land on the same endpoint.

## Mode 1 — mDNS auto-discovery (default)

Server side (defaults; nothing to configure):

```
SPAXEL_MDNS_ENABLED=true          # default
SPAXEL_MDNS_NAME=spaxel           # default
# SPAXEL_ADVERTISED_BASE_URL      # optional; see "Diagnostics" for failure modes
```

On startup the mothership logs:

```
[INFO] mDNS advertising spaxel._spaxel._tcp.local:<port>
```

The advertisement carries TXT records `version=1`, `ws=/ws/node`,
`dashboard=/ws/dashboard`.

Workflow:

1. Run the mothership on the same L2 network as the nodes. In Docker this
   **requires `network_mode: host`** — see "Networking requirements".
2. Open the dashboard → onboarding wizard. The "mothership host" field
   defaults to the browser's hostname and becomes the payload's `ms_mdns`.
3. Plug the node in over USB; the wizard provisions WiFi credentials (from
   Settings → Network by default, ADR-005), `node_token`, `ms_mdns`,
   `ms_port` and writes them to NVS.
4. Reboot the node. It joins WiFi and resolves `ms_mdns` via mDNS.

### Node-side resolution order (per discovery cycle)

`main.c` `NODE_STATE_MOTHERSHIP_DISCOVERY` tries, in order:

1. **Provisioned IP override** (`ms_ip` in NVS), if present — mDNS is skipped
   on the first attempt.
2. **mDNS** — `wifi_discover_mothership` issues a PTR query for
   `_spaxel._tcp.local` (5 s timeout) and accepts the result whose hostname
   contains the provisioned instance name; port comes from the SRV record.
3. **Cached IP** — the last IP a successful WebSocket connection used
   (persisted to NVS on every connect), so a mothership that keeps its
   address keeps its nodes across an mDNS outage.
4. **Give up for the cycle** — after 10 failed attempts (5 s apart) the node
   logs "Mothership unavailable, continuing in degraded mode" and retries.

## Mode 2 — manual IP fallback (mDNS-less networks)

Use when multicast is unavailable: WiFi APs with multicast/broadcast
filtering enabled, Docker bridge networks, or mixed-VLAN deployments.

Server side:

```
SPAXEL_MDNS_ENABLED=false
```

Nothing is advertised (the startup log stays free of `mDNS advertising`
lines) and the doctor's `mdns_binding` check reports `ok` while disabled —
this is a supported configuration, not a fault.

Workflow:

1. Start the mothership with `SPAXEL_MDNS_ENABLED=false`.
2. Note the mothership's LAN IP and the port it listens on
   (`SPAXEL_BIND_ADDR`).
3. In the onboarding wizard, enter that IP in the "mothership IP" field. The
   browser pre-fills it automatically when you are already browsing the
   mothership by IP. It is sent as `ms_ip` in the provision request.
4. The payload stores `ms_ip` twice in NVS (`ms_ip` and the provisioned
   override), so the node dials the IP first every boot; mDNS is only
   attempted as a later fallback if the IP stops answering.

A payload from this mode still carries `ms_mdns` — it costs nothing and keeps
the node discoverable if it later lands on an mDNS-capable segment. The
manual mode's essence is the extra `ms_ip` field, not the absence of
`ms_mdns`.

## Networking requirements

The plan's anti-pattern: **a Docker bridge network drops multicast
(224.0.0.251)**, so a bridged mothership is undiscoverable no matter how
correctly it advertises. Exactly one of these must hold (asserted by
`TestComposeHostNetworkingContract`):

- `network_mode: host` in `docker-compose.yml` (current state), or
- `SPAXEL_MDNS_ENABLED=false` — explicit manual-IP mode.

WiFi-side: multicast/broadcast filtering on the AP blocks mDNS the same way;
switch to Mode 2 rather than weakening the AP config.

## Failure diagnostics

### Server side — `GET /api/doctor` (auth required)

The `mdns_binding` check:

| Condition                        | Status | Message                                              |
|----------------------------------|--------|------------------------------------------------------|
| `SPAXEL_MDNS_ENABLED=false`      | `ok`   | (empty — supported configuration)                    |
| enabled and advertising          | `ok`   | (empty)                                              |
| enabled and NOT advertising      | `warn` | `mDNS not advertising — nodes cannot auto-discover mothership` |

### Server side — startup logs

| Log line | Meaning / action |
|---|---|
| `mDNS advertising <name>._spaxel._tcp.local:<port>` | Advertisement is up; `<port>` is the actual listener port |
| `Could not bind mDNS to the node-reachable address "<url>": ...` | WARN: `SPAXEL_ADVERTISED_BASE_URL` does not resolve to a local interface (multi-homed host, or a non-routable URL). Advertisement falls back to the system multicast interface; check that the fallback interface is the one the nodes are on, or set the URL to the LAN address |
| `SPAXEL_ADVERTISED_BASE_URL=... invalid: "0.0.0.0" is a wildcard bind address, not routable from nodes` | FATAL — startup refuses to continue. Advertised URLs must name a concrete LAN address or host name |

### Node side — serial log

| Log line | Meaning |
|---|---|
| `Trying provisioned mothership IP: <ip>:<port>` | Manual-IP mode path taken first |
| `Querying mDNS for <name>.<service>.local:<port>` | mDNS attempt starting |
| `mDNS query failed or no results` | No advertisement seen — server side down, multicast filtered, or different L2 |
| `Found mothership: <ip>:<port>` / `Using first mDNS result: ...` | Discovery succeeded |
| `Using cached mothership IP: <ip>` | mDNS missed, cached address in use |
| `Mothership discovery failed (attempt N)` | Retry cycle (5 s apart, 10 attempts) |
| `Mothership unavailable, continuing in degraded mode` | Ten consecutive failures; discovery keeps retrying from this state |

The fastest triage split: if the node logs `mDNS query failed` **and** the
server's doctor says `warn`, the advertisement is down; if the doctor says
`ok` but the node still misses it, the network is filtering multicast —
switch to Mode 2.

## Reconnect behavior

Connection state on the mothership is authoritative: a dropped WebSocket
marks the node offline immediately (no last-seen grace window), and the next
`hello` marks it online again. Nodes reconnect on their own — the token and
`ms_mdns`/`ms_ip` written at provision time stay valid across reboots and
reconnects, so no re-provisioning step exists or is needed. Covered by
`TestNodeReconnectAfterDisconnect`.

## Verifying discovery from a workstation

Same L2 as the mothership:

```bash
# Linux (Avahi)
avahi-browse -r _spaxel._tcp
# macOS
dns-sd -B _spaxel._tcp
```

Expected: one instance named after `SPAXEL_MDNS_NAME` (default `spaxel`),
with SRV port equal to the mothership's HTTP port and TXT
`version=1|ws=/ws/node|dashboard=/ws/dashboard`.

## Test coverage

`mothership/tests/e2e/mdns_discovery_test.go` drives a real mothership
subprocess per scenario:

- `TestMDNSDiscoveryEnabled` — live mDNS browse finds the instance; the
  advertised SRV port equals the HTTP listener (regression guard: the
  advertisement once hardcoded `:8080`); TXT carries the WS paths; the
  provisioning payload agrees; doctor reports `ok`.
- `TestMDNSDiscoveryDisabled` — nothing on the wire (verified against a
  control advertisement that proves the browse itself works); doctor `ok`;
  payload honors the `ms_ip` override.
- `TestAdvertisedBaseURLDiagnostics` — non-routable URL degrades to the WARN
  with a working fallback advertisement; wildcard URL is a fatal startup
  error.
- `TestNodeReconnectAfterDisconnect` — offline on disconnect, online again on
  redial with the same token, no re-provisioning.
- `TestComposeHostNetworkingContract` — the compose file keeps host
  networking or explicitly disables mDNS.

Discovery assertions are anchored by a control mDNS service the test
advertises itself; on a host without working multicast they skip rather than
report a false negative (the HTTP- and log-level assertions still run).
