# Mothership Dashboard — Dependencies and Prerequisites

**Date:** 2026-09-04
**Bead:** spaxel-e92e73bf
**Verified against:** `main` @ `2f8a6e270d857aa10164dc820445426c6a2ad568`
**Sibling deliverables:** `MOTHERSHIP_DASHBOARD_LOCATIONS.md`, `MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md` (startup command / entry point / serving modes)

All `file:line` citations below were re-confirmed with `git grep`/`git show HEAD:` at the
revision above, not from a stale or dirty working tree.

---

## Executive summary

The dashboard is not a separate process — it is routes on the mothership's HTTP server
(`func main()` `mothership/cmd/mothership/main.go:703`), so **"dashboard dependencies" are
mothership dependencies plus a browser**. The dependency surface is deliberately small:

- **No external database server, no Redis, no message queue, no cloud relay.** All state
  lives in two local SQLite files created in `SPAXEL_DATA_DIR`.
- **No required configuration.** The binary reads no config file and accepts no CLI flags;
  all 28 environment variables have working defaults.
- **No required credentials.** Nothing needs a token, key, or password to boot.
- **One port** (TCP 8080) plus mDNS multicast for node discovery; everything else is
  optional outbound integrations.

---

## 1. Runtime dependencies — databases, message queues, external APIs (AC1)

### 1.1 Databases

| Database | Created at | Driver | Notes |
|---|---|---|---|
| `<SPAXEL_DATA_DIR>/spaxel.db` | `main.go:758` (`db.OpenDB`) | `modernc.org/sqlite v1.47.0` | main store — fleet, settings, auth/`sessions`, events, zones |
| `<SPAXEL_DATA_DIR>/automation.db` | `main.go:1023` (`automation.NewEngine`) | same | separate DB for the automation engine |

Both are **embedded, pure-Go SQLite** — no `libsqlite3`, no server process, and the binary
is built `CGO_ENABLED=0` (`Dockerfile:56`). No other persistence exists; there is no
postgres/mysql/redis anywhere in `go.mod`.

### 1.2 Message queues

**None required.** The only queue-shaped dependency is an **optional outbound MQTT
client** (`github.com/eclipse/paho.mqtt.golang v1.5.0`) for Home Assistant discovery:

- Created only when `SPAXEL_MQTT_BROKER` is set — `main.go:1462`
  (`if cfg.MQTTBroker != "" { mqttClient, err = mqtt.NewClient(...) }`).
- Discovery prefix `homeassistant`, `AutoReconnect: true` (`main.go:1463-1475`).
- No broker is embedded; unset broker = the integration is simply off.

### 1.3 External APIs (all optional, all outbound)

| API | When contacted | Required? | Evidence |
|---|---|---|---|
| GitHub REST API | Startup `Ping` for the Kaniko release checker; later release polling | No — client is constructed unconditionally (`main.go:1035-1054`) but a ping failure only logs `[WARN]` and startup continues. Unauthenticated = 60 req/h, `SPAXEL_GITHUB_TOKEN` raises it to 5000 req/hour | `main.go:1041-1047` |
| ntfy | Push notifications, only if a ntfy channel is configured in Settings | No. Default endpoint `https://ntfy.sh`; self-hosted URL supported | `internal/notifications/ntfy.go:16` |
| Pushover | Push notifications, only if configured | No. `https://api.pushover.net/1/messages.json` | `internal/notifications/pushover.go:74`, `internal/notify/service.go:507` |
| Generic webhooks | Only if a webhook target is configured | No. Plain `http.Client` POST | `internal/webhook/publisher.go:39,54` |
| IEEE OUI database | **Build-time only** — `go:generate` fetches `https://standards-oui.ieee.org/oui/oui.txt` and embeds the table. Not contacted at runtime | No | `internal/oui/gen_data.go:25` |
| NTP | The mothership itself never calls out; **ESP32 nodes** sync to `pool.ntp.org` by default. `SPAXEL_NTP_LOCAL_ENABLED=true` makes the mothership serve time instead (UDP 123) | No | `internal/ntpserver/server.go:47-54` |
| GitHub Releases (firmware) | **Image-build time only** — `curl` of `spaxel-firmware-${VERSION}-merged.bin` into the image | Required to *build the image*, not to run it | `Dockerfile:10-26` |

**Not a dependency at all:** no S3/B2, no auth provider at the application layer (§4), no
telemetry/phone-home, no container registry at runtime.

---

## 2. Network requirements (AC2)

### 2.1 Inbound listeners

| Port / protocol | Purpose | Required? | Evidence |
|---|---|---|---|
| TCP 8080 (HTTP/1.1 + WebSocket upgrade) | The dashboard, `/healthz`, all `/api/*`, `/ws/dashboard`, node endpoint `/ws/node`, firmware download `/firmware/*` | **Yes — the only required listener.** Single `http.Server` on `SPAXEL_BIND_ADDR`, default `0.0.0.0:8080` | `config.go:92`, `Dockerfile:96` (`EXPOSE 8080`), `/healthz` route `main.go:800` |
| UDP 5353 → multicast 224.0.0.251 | mDNS advertisement `_spaxel._tcp.local` so nodes auto-discover the mothership | Effectively yes for node onboarding; disable with `SPAXEL_MDNS_ENABLED=false` (nodes then need a cached/manual IP) | `config.go:129`, `docker-compose.yml:23` (`network_mode: host` — **required**, bridge networking blocks multicast) |
| UDP 123 | Embedded SNTP server for nodes | No — only when `SPAXEL_NTP_LOCAL_ENABLED=true`; binding a privileged port needs `cap_add: NET_BIND_SERVICE` in the distroless container | `config.go:245`, `docker-compose.yml:36-37`, `internal/ntpserver/server.go:54` |

### 2.2 Outbound

| Destination | Trigger |
|---|---|
| MQTT broker, typically 1883/8883 | `SPAXEL_MQTT_BROKER` set |
| HTTPS 443 — github.com API | always attempted, WARN-only on failure |
| HTTPS 443 — ntfy / Pushover / webhook targets | respective notification channel configured |
| HTTPS 443 — GitHub Releases | image build only |

### 2.3 Browser → dashboard

`/` plus `/ws/dashboard` WebSocket (10 Hz position blobs + events). A reverse proxy in front
must allow long-lived WebSocket connections — the compose file's commented Traefik block
extends read timeout to 3600 s for exactly this reason (`docker-compose.yml:91-93`).

---

## 3. Configuration files and environment variables (AC3)

### 3.1 There is no config file

The mothership reads **no configuration file at all** — no YAML/JSON/TOML, no `.env`
(compose interpolates `${VAR:-default}` from the shell environment, `docker-compose.yml`).
It also accepts **no CLI flags**: `main.go:703` goes straight into `config.Load()`, and the
only `flag.NewFlagSet` in the package (`cmd/mothership/migrate.go:40`) sits behind
`//go:build ignore_migrate` (`migrate.go:1`) and is **not part of the shipped binary**.

The files that *do* configure a deployment:

| File | Role |
|---|---|
| `docker-compose.yml` | deployment shape — host networking, volumes, caps, limits, which env vars to set |
| `Dockerfile` | build; copies `dashboard/` into `cmd/mothership/dashboard/` and embeds it with `-tags=embed` (`Dockerfile:46,60`) |
| `dashboard/` assets | served as-is; no build step. Production = embedded; dev run falls back to `{"./dashboard", "./../dashboard", "/app/dashboard"}` (`main.go:295`) or `SPAXEL_STATIC_DIR` |
| `$SPAXEL_DATA_DIR/` | created at runtime: `spaxel.db`, `automation.db`, firmware OTA store, floor plans, CSI replay buffer |

### 3.2 Complete environment-variable surface (all optional)

Every variable below is read in `mothership/internal/config/config.go`; line numbers are at
HEAD `2f8a6e27`. **None is required** — an empty environment boots a working dashboard on
`:8080`.

| Variable | Default (config.go) | Effect |
|---|---|---|
| `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` (:92) | HTTP + WebSocket listen address — the dashboard's port/host |
| `SPAXEL_ADVERTISED_BASE_URL` | derived (:97-101) | Base URL handed to nodes for OTA/mDNS (ADR-004); must be routable *from nodes*, not the bind address |
| `SPAXEL_DATA_DIR` | `/data` (:119) | SQLite DBs, firmware store, floor plans, replay buffer |
| `SPAXEL_STATIC_DIR` | `/dashboard` (:122) | Dev-only override for dashboard assets (embedded FS wins in `embed` builds) |
| `SPAXEL_SEED_FIRMWARE_DIR` | `/firmware` (:126) | Seed source copied into the OTA store on first run |
| `SPAXEL_MDNS_ENABLED` | `true` (:129) | `_spaxel._tcp.local` advertisement; needs host networking |
| `SPAXEL_MDNS_NAME` | `spaxel` (:139) | mDNS instance name (must match the node's `ms_mdns` NVS key) |
| `SPAXEL_LOG_LEVEL` | `info` (:142) | Log verbosity (`debug` for troubleshooting) |
| `SPAXEL_LOG_FILE_PATH` | unset (:148) | Optional file log sink |
| `SPAXEL_LOG_STDOUT` | `true` (:151) | Stdout log sink |
| `SPAXEL_FUSION_RATE_HZ` | default (:161) | Position fusion rate |
| `SPAXEL_REPLAY_MAX_MB` | default (:176) | CSI replay buffer cap |
| `SPAXEL_REPLAY_COMPRESSION` | default (:191) | Replay compression codec |
| `SPAXEL_REPLAY_CHUNK_MB` | default (:203) | Replay chunk size |
| `SPAXEL_INSTALL_SECRET` | auto-generated (:218) | Node provisioning HMAC secret — persisted in DB on first run, overridable once at first boot |
| `SPAXEL_MIGRATION_WINDOW_HOURS` | default (:234) | Node auth-token migration window |
| `SPAXEL_NTP_LOCAL_ENABLED` | `false` (:245) | Serve SNTP to nodes (UDP 123) |
| `SPAXEL_NTP_SERVER` | unset (:258) | Override the NTP server advertised to nodes |
| `SPAXEL_MQTT_BROKER` | unset (:271) | Enables the optional MQTT/HA integration |
| `SPAXEL_MQTT_USERNAME` | empty (:284) | Broker credentials |
| `SPAXEL_MQTT_PASSWORD` | empty (:287) | Broker credentials |
| `SPAXEL_WIFI_SSID` | empty (:290) | First-boot seed only (ADR-005); the DB setting is authoritative afterwards |
| `SPAXEL_WIFI_PASSWORD` | empty (:293) | First-boot seed only |
| `SPAXEL_GITHUB_TOKEN` | empty (:296) | GitHub API rate limit 5000 vs 60 req/h |
| `SPAXEL_DEMO_MODE` | `false` (:299) | Blocks mutating HTTP methods (§4) |
| `SPAXEL_MAX_DASHBOARD_CLIENTS` | default (:309) | WebSocket dashboard-client cap |
| `TZ` | UTC (:324) | Diurnal baselines, briefings, quiet hours |

Compose adds its own interpolation variables: `VERSION`, `TZ`, `SPAXEL_MDNS_NAME`,
`SPAXEL_MDNS_ENABLED`, `SPAXEL_NTP_LOCAL_ENABLED`, `TRAEFIK_ENABLE`
(`docker-compose.yml:18,41-49,86`).

**Known trap:** `SPAXEL_FIRMWARE_DIR` is read by no code — the real variable is
`SPAXEL_SEED_FIRMWARE_DIR`. Setting the former is a silent no-op.

---

## 4. Authentication and credential requirements (AC4)

### 4.1 Nothing is required to boot

The binary starts and serves the dashboard with **zero credentials**. `SPAXEL_INSTALL_SECRET`
is auto-generated and persisted if not supplied; GitHub/MQTT/notification credentials are
all optional enhancements.

### 4.2 What actually gates dashboard access (ground truth at HEAD)

The chi middleware chain is exactly three entries — Logger, Recoverer,
`auth.DemoModeMiddleware` (`main.go:748-753`):

- **`auth.Handler.Middleware` — the PIN/session gate — is defined but never mounted.** It
  exists at `internal/auth/handler.go:793` with no call sites; the only `r.Use(auth.…)` in
  the codebase is DemoModeMiddleware (`main.go:753`).
- The code says why: *"Auth is handled at the Traefik layer (Google OAuth) — no in-app PIN
  auth."* (`main.go:775`).
- **Operational consequence:** anyone who can reach TCP 8080 can read the dashboard and call
  unauthenticated API routes. Deployments must put an authenticating reverse proxy (Traefik
  forward-auth / OAuth) or network isolation in front of the port. The compose file's
  Traefik labels are commented out by default (`docker-compose.yml:85-93`), so **a stock
  `docker compose up -d` is unauthenticated**.

What does exist and works:

| Mechanism | Scope | Evidence |
|---|---|---|
| PIN setup/login/logout, session cookie `spaxel_session` (HttpOnly, SameSite=Strict, 7-day) | Endpoints live and session store works, but they are **not a gate** — `RequireAuth` is applied only to `POST /api/auth/change-pin` (`handler.go:188,200`) and `GET /api/doctor` (`main.go:5053`) | `handler.go:532-538` (cookie), `:601` (RequireAuth) |
| `IsPublicPath` allowlist (`/healthz`, `/api/auth/*`, `/api/provision`, `/ws/node`, `/firmware/*`) | Only meaningful inside the unmounted middleware | `handler.go:703` |
| Demo mode (`SPAXEL_DEMO_MODE=true`) | Rejects POST/PUT/PATCH/DELETE globally with 403 | `handler.go:730-748` |
| Node authentication | Real and enforced: `/ws/node` and `/firmware/*` require `X-Spaxel-Token`, validated as HMAC of the install secret via `provSrv.ValidateToken` | `main.go:4837` (ingest), `main.go:4857` (OTA) |

### 4.3 Credential inventory

| Credential | Needed? | Where it lives |
|---|---|---|
| Install secret (node provisioning) | Auto-generated | `auth` table in `spaxel.db`; override once via `SPAXEL_INSTALL_SECRET` |
| MQTT username/password | Only if `SPAXEL_MQTT_BROKER` set | env (`config.go:284,287`) |
| GitHub token | No | env (`config.go:296`) |
| ntfy / Pushover / webhook tokens | Only if those channels configured | Dashboard Settings → stored in `spaxel.db` (not env) |
| WiFi SSID/password | No (node provisioning) | DB setting; env only seeds first boot |
| TLS certs | No | Terminate TLS at the reverse proxy |

Secrets should reach the container as environment references (compose does not read a
`.env`-file path by itself unless one sits next to the compose file), never baked into the
image.

---

## 5. System dependencies (AC5)

### 5.1 Runtime (production container)

| Requirement | Detail |
|---|---|
| Docker + BuildKit/buildx | Multi-stage, `--platform` build args (`Dockerfile:4-5,33`); `spaxel-build` Argo template does this in CI |
| Nothing else | Runtime image is `gcr.io/distroless/static-debian12:nonroot` (`Dockerfile:73`) — no shell, no package manager, no libc needed (static binary, `CGO_ENABLED=0`, `Dockerfile:56`). Runs as UID 65532. `ENTRYPOINT ["/spaxel"]` (`Dockerfile:99`), `VOLUME /data` (`Dockerfile:93`) |
| Host networking | Required for mDNS (`docker-compose.yml:23`) |
| Kernel capabilities | `NET_BIND_SERVICE` only if the local NTP server is enabled (`docker-compose.yml:36-37`) |
| Resources (compose defaults) | 512 MB RAM / 2.0 CPUs limit (raise to 1 GB for 16+ node fleets), 128 MB / 0.5 CPU reservation, `nofile` 4096/8192, `stop_grace_period: 35s` (`docker-compose.yml:62-81`) |
| Named volume | `spaxel-data` → `/data`; optional bind `./firmware:/firmware:ro` (`docker-compose.yml:29-31`) |
| Free TCP 8080 on the host | Host networking means no `ports:` mapping and no remap |

**Caveat:** the compose `healthcheck` invokes `wget`
(`docker-compose.yml:68`), which does not exist in the distroless runtime image — the
healthcheck as committed cannot execute there. The service itself is unaffected; probe
`/healthz` from outside the container instead.

### 5.2 Building from source (no Docker)

| Requirement | Detail |
|---|---|
| Go ≥ 1.25.0 | `mothership/go.mod:3` (`go 1.25.0`); builder image `golang:1.25-bookworm` (`Dockerfile:33`) |
| No C toolchain | `CGO_ENABLED=0` everywhere; SQLite is pure Go. `go build -tags=embed -o spaxel ./cmd/mothership` after copying `dashboard/` into `cmd/mothership/dashboard/` |
| One-time network access | `go mod download`; firmware artifacts only for the container path (`Dockerfile:10-26`) |
| ESP-IDF | **Not required** — firmware is downloaded prebuilt from GitHub Releases, never compiled locally |

### 5.3 Development / test tooling (not needed to serve the dashboard)

| Requirement | Detail |
|---|---|
| Node 20 | Only pin in the repo: CI a11y gate `image: node:20-bookworm-slim` (`dashboard/README.md:30`). `dashboard/package.json` has **no `engines` field and no `.nvmrc`** |
| Dev dependencies | `jest` 29.7 + `jest-environment-jsdom` (unit), `@playwright/test` 1.52 + `@axe-core/playwright` 4.10.1 (`npm run test:a11y`, needs `npx playwright install --with-deps chromium`), `typescript` 6.0.3 (`npm run typecheck`), `http-server` 14.1.1 |
| No Python | No `requirements.txt`, no Python tooling anywhere in the dashboard build |

### 5.4 Client (browser)

Any modern browser with WebSocket support. Three.js 3D view needs WebGL; **Web Serial
(Chromium-based)** is required only for the USB "Add Node" provisioning path.

---

## 6. Minimal-prerequisite checklist

To run the dashboard with nothing optional enabled:

- [x] Docker with BuildKit (or Go 1.25+ for a dev run)
- [x] One free TCP port (default 8080) on the host
- [x] A writable volume at `/data`
- [x] Host networking (or accept nodes' cached-IP fallback with `SPAXEL_MDNS_ENABLED=false`)
- [x] A reverse proxy with authentication in front of 8080 — **not enforced by the app** (§4.2)
- [ ] Nothing else: no config file, no env vars, no credentials, no external services

---

## 7. Cross-references

- Startup command, entry point, 7-phase sequence, serving modes: `MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md` (§12 re-verifies its own line cites at `3eb3eb42`; the config.go lines there are unchanged at `2f8a6e27`)
- Dashboard asset locations and embed mechanism: `MOTHERSHIP_DASHBOARD_LOCATIONS.md`
- Auth-layer decision (Traefik/Google OAuth vs in-app PIN): `docs/plan/plan.md` ADR-006; first-boot WiFi seeding ADR-005; advertised base URL ADR-004
