# Network Boundary — Verified Contract

**Date:** 2026-09-26
**Purpose:** Document the verified network boundary of the mothership: what listens, what dials out, where detection data can and cannot flow, and how each claim is mechanically verified. Companion to `internal/privacy/doc.go` (the source-level guarantee) and the live verification in `mothership/tests/e2e/privacy_network_boundary_test.go`.

## The contract

Detection is local-first: CSI, presence and localization data stay on the operator's LAN. The mechanically checkable form:

1. **One inbound surface.** The mothership exposes exactly one TCP listener, at `SPAXEL_BIND_ADDR`, serving the HTTP API, WebSocket feeds and the dashboard. The harness verifies the count is exactly one.
2. **No detection-data egress.** The detection path (ingestion, fusion, api, dashboard, recorder, tracking) contains zero outbound dial sites — enforced statically — and a live run under full simulated load holds zero non-loopback sockets — enforced at runtime. A cloud relay cannot exist without failing one of the two.
3. **Bind-only LAN services** (never relays): the mDNS advertisement (UDP/5353, `SPAXEL_MDNS_ENABLED`, default on) and the opt-in embedded SNTP responder (UDP/123, `SPAXEL_NTP_LOCAL_ENABLED`, default off). Both bind and answer; neither connects anywhere.

## Inbound surface

- Single `http.Server` on `SPAXEL_BIND_ADDR` (`cmd/mothership/main.go`, startup phase 7 also self-checks `/healthz` over loopback).
- Per-route auth expectations are pinned by `internal/auth/route_surface_test.go`, which walks the live router surface against `IsPublicPath` (`internal/auth/handler.go`). Note (by design): the in-app PIN middleware is intentionally not installed on the production router — access control terminates upstream at the Traefik forward-auth layer in the reference deployment.
- The mDNS advertisement publishes the dashboard service on the LAN; it is discovery only.

## Outbound traffic — measured at HEAD (2026-09-26)

**Static guarantee.** `internal/privacy/egress_allowlist_test.go` scans every non-test `.go` file compiled into the mothership and fails on any outbound-dial construct outside an explicit, commented allowlist of operator-configured integration packages (notifications, webhooks/automation, MQTT broker, doctor probes, GitHub release lookups). The detection-path packages must contain zero dial sites outright.

**Live finding — the mothership is not fully airgapped by default.** Startup phase 5 initializes the GitHub API client (used for Kaniko release lookups) and performs an availability ping of its endpoint — `api.github.com` by default — **even when completely unconfigured**:

```
[CONFIG] SPAXEL_GITHUB_TOKEN=(not set, unauthenticated GitHub API requests will be rate-limited)
[INFO] GitHub API client endpoint: https://api.github.com
[GITHUB] API ping successful (status 401)
[INFO] GitHub API client initialized (unauthenticated, rate limit: 60 req/hour)
[PHASE 5/7] Subsystem GitHub API client started
```

The ping carries no payload (no detection data, no credentials beyond an optional operator-supplied token) and a failed ping is non-fatal (WARN + continue). But it is unconditional: a stock deployment contacts api.github.com once at startup. **To airgap, set `SPAXEL_GITHUB_API_URL`** to an internal mirror (e.g. a Forgejo release endpoint) or any loopback address; the client target is pure operator configuration.

With that knob pointed at loopback, the complete socket inventory of a running mothership under load is:

| Socket | Purpose | Remote |
|---|---|---|
| TCP LISTEN `SPAXEL_BIND_ADDR` | HTTP API + WS + dashboard | — (inbound) |
| TCP ESTABLISHED (transient, many) | node WS feeds, API polls, self-check | 127.0.0.1 only |
| UDP 0.0.0.0:5353 + [::]:5353 | mDNS advertisement (unconnected) | — (bind-only) |
| UDP :123 | SNTP responder (unconnected, opt-in) | — (bind-only) |

Zero other non-loopback sockets appear across continuous sampling of the whole run.

## How each claim is verified

Three layers, all in the normal `go test ./...` run:

1. **Static source scan** — `internal/privacy/egress_allowlist_test.go`: no dial site outside the allowlist; detection path has none at all; allowlist entries must remain non-stale.
2. **Live process observation** — `mothership/tests/e2e/privacy_network_boundary_test.go` (`TestMothershipNetworkBoundaryStaysLocal`): starts a real mothership + simulator, streams CSI while blobs are produced, and samples the process's sockets ~5×/second for the whole run (double `/proc/<pid>/fd` inode read around each `/proc/net/{tcp,udp}{,6}` read, to exclude inode-recycling false positives). Asserts: exactly one TCP listener at the configured bind address; every other TCP socket loopback-only; no connected UDP to a non-loopback unicast peer (kernel-DNS port 53 exempted); sanity guards ensure ≥20 samples and ≥1 observed loopback connection so a broken scanner fails loudly instead of passing vacuously; and blobs must be flowing during the window so the boundary is proven under load, not against an idle server. Run it alone with `cd mothership && go test ./tests/e2e/ -run TestMothershipNetworkBoundaryStaysLocal -v`. Linux-only (skips elsewhere); the harness points `SPAXEL_GITHUB_API_URL` at a closed loopback port so the one known dialer is deflected and the run proves *nothing else* dials out.
3. **Route-level behavior** — `internal/auth/route_surface_test.go` pins every route's auth expectation on the live router.

## Residual limits (honest scope)

- The live test observes sockets, not payloads: it proves no non-loopback connection *exists*, which is strictly stronger than "no detection data in outbound payloads" for the default posture.
- The GitHub ping means "zero cloud contact" requires the documented `SPAXEL_GITHUB_API_URL` override; without it, one payload-free contact to api.github.com happens per boot.
- The SNTP responder (if enabled) answers LAN time queries over UDP/123; it is off by default.
- Physical-layer WiFi telemetry (CSI frames from ESP32 nodes) arrives over the LAN by construction; the mothership never initiates node connections.
