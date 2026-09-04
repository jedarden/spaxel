# Mothership Dashboard — Complete Startup Procedure

**Date:** 2026-09-04
**Bead:** spaxel-1cae2893 (synthesis bead — compiles the family's findings into one operating procedure)
**Verified against:** `main` @ `95041178` — `mothership/`, `Dockerfile`, `docker-compose.yml` and `VERSION` are byte-identical to `2f8a6e27` / `3eb3eb42` (`git diff --stat 2f8a6e27..HEAD` over those paths is empty), so the line cites in the sibling deliverables remain valid here.
**Sibling deliverables:** `MOTHERSHIP_DASHBOARD_LOCATIONS.md` (asset locations), `MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md` (startup method + env surface), `MOTHERSHIP_DASHBOARD_DEPENDENCIES.md` (dependency/prerequisite inventory)

Every `file:line` below was confirmed against `git show HEAD:` at the revision above, not
against this working tree (which carries another worker's in-flight edits to `main.go` and
`config.go`).

---

## 0. What you are actually starting

The dashboard is **not a separate process**. It is a set of routes on the mothership's
single HTTP server — `func main()` at `mothership/cmd/mothership/main.go:703`, assets
embedded into the binary via `//go:embed dashboard` (`mothership/cmd/mothership/dashboard_embed.go`,
`-tags=embed` at `Dockerfile:60`). Starting the dashboard *is* starting the mothership.

Three facts that shape every step below:

1. **No CLI flags exist.** `main()` goes straight into `config.Load()`; the only
   `flag.NewFlagSet` in the package (`cmd/mothership/migrate.go:40`) sits behind
   `//go:build ignore_migrate` and is not part of the shipped binary. Configuration is
   environment-only (`mothership/internal/config/config.go`).
2. **No environment variable is required.** Every knob has a working default; an empty
   environment boots a working dashboard on `0.0.0.0:8080` (`config.go:92`).
3. **Nothing is required to authenticate.** `SPAXEL_INSTALL_SECRET` is auto-generated and
   persisted on first run. The dashboard PIN/session middleware is defined
   (`internal/auth/handler.go:793`) but **never mounted** — the chi chain is exactly
   Logger, Recoverer, `DemoModeMiddleware` (`main.go:750-753`), because "Auth is handled at
   the Traefik layer (Google OAuth) — no in-app PIN auth" (`main.go:775`). **A stock start
   is unauthenticated** — see §7.1 before exposing the port anywhere.

---

## 1. Prerequisites

| # | Requirement | Detail | Only for |
|---|---|---|---|
| 1 | Docker + BuildKit, **or** Go ≥ 1.25.0 | `Dockerfile:33` builder image; `mothership/go.mod:3` `go 1.25.0` | both paths |
| 2 | One free TCP port (default 8080) on the host | Host networking means **no `ports:` remap is possible** (`docker-compose.yml:9-10,23`) | both |
| 3 | A writable data directory | Default `/data` (`config.go:119`); compose provides the `spaxel-data` volume (`docker-compose.yml:30`). A dev run must override it — see §3 step 2 | both |
| 4 | Host networking (or a deliberate mDNS opt-out) | mDNS multicasts to 224.0.0.251, which Docker bridge blocks (`docker-compose.yml:4-10`). Alternative: `SPAXEL_MDNS_ENABLED=false` and provision nodes with a cached/manual IP | node discovery only |
| 5 | Firmware binaries for onboarding/OTA | Baked into the CI image; see §2 step 3 for the from-source trap | node onboarding only |
| 6 | A browser | Any modern browser; WebGL for the 3D view; Web Serial (Chromium) only for USB "Add Node" | UI only |
| 7 | An authenticating reverse proxy **if the port leaves the host** | Not enforced by the app (§0 fact 3) | exposure only |

Nothing else: no config file, no `.env`, no external database, no message queue, no
required credentials. `mothership/internal/config/config.go` is the complete
configuration surface (28 env vars, all optional — table in
`MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md` §12).

---

## 2. Path A — Docker Compose (recommended)

### Step 1 — Get the repo

```bash
git clone https://git.ardenone.com/jedarden/spaxel.git && cd spaxel
```

### Step 2 — Choose the image source (this decides everything else)

`docker-compose.yml` at HEAD **builds locally** — `build:` at `docker-compose.yml:14-18`
with `VERSION: ${VERSION:-dev}`; the prebuilt-image line is **commented out**
(`# image: ghcr.io/spaxel/spaxel:latest`, `docker-compose.yml:20`). So a bare
`docker compose up -d` compiles the Go binary from your checkout and pulls the ESP32
firmware from GitHub Releases inside the build (§2 step 3). Plan.md's and
`MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md` §5's "run `ghcr.io/spaxel/spaxel:latest`"
describes the commented-out alternative, not what the file does.

- **Production / prebuilt (fast, no toolchain, no network fetch of firmware):** flip the
  image line on and skip the build —
  ```bash
  # docker-compose.yml:20  — uncomment:
  #   image: ghcr.io/spaxel/spaxel:latest
  # and remove the build: block at :14-18
  docker compose up -d
  ```
  The published image is `ronaldraygun/spaxel` (built by the `spaxel-build` Argo
  WorkflowTemplate); pin a tag, never `:latest`, in a real deployment.
- **From source (default file as committed):** continue to step 3.

### Step 3 — If building from source: satisfy the firmware-fetch stage

Dockerfile stage 1 is a **fetcher, not a firmware build** (`Dockerfile:10-26`): it
`curl`s `spaxel-firmware-${VERSION}-merged.bin` from
`github.com/jedarden/spaxel/releases/download/v${VERSION}/…`. With the compose default
`VERSION=dev` that URL does not exist and **the build fails with a 404 in the
firmware-fetcher stage**. Pass a version that has a published release:

```bash
VERSION="$(cat VERSION)" docker compose up -d --build
```

`VERSION` at HEAD is `0.2.161`. If that release is not published on GitHub, use the
prebuilt image (step 2) instead — do not hand-edit the fetch URLs.

### Step 4 — Firmware mount check (one-time)

Compose bind-mounts `./firmware:/firmware:ro` (`docker-compose.yml:31`), which **shadows**
the firmware baked into the image. On first boot the mothership copies
`$SPAXEL_SEED_FIRMWARE_DIR` (`/firmware`, `config.go:126`) into `<data>/firmware/`
(`seedFirmwareDir`, `main.go:5753`; invoked at `main.go:4707`) and logs
`[INFO] firmware seed: installed <file>`. An empty `./firmware/` therefore seeds nothing
and OTA / Web Serial onboarding has no payload. Either place `*.bin` files there or delete
the bind mount to use the image's baked firmware.

### Step 5 — Set environment (all optional)

Compose interpolates its own four variables from the shell
(`docker-compose.yml:18,41-49,86`): `VERSION` (default `dev`), `TZ` (default `UTC`),
`SPAXEL_MDNS_NAME` (`spaxel`), `SPAXEL_MDNS_ENABLED` (`true`),
`SPAXEL_NTP_LOCAL_ENABLED` (`false`), `TRAEFIK_ENABLE` (`false`). Uncomment lines in the
`environment:` block for MQTT (`:51-53`) or debug logging (`:55`).

The only variable that changes **where the dashboard answers** is `SPAXEL_BIND_ADDR`
(default `0.0.0.0:8080`, `config.go:92`). With host networking, `127.0.0.1:8080` confines
the dashboard to the host loopback.

### Step 6 — Start

```bash
docker compose up -d
docker compose logs -f spaxel      # expect §5's log sequence
```

Startup is a 7-phase sequence under one 30-second deadline
(`startup.TotalTimeout`, `mothership/internal/startup/startup.go:18`); a phase failure is
fatal, not a retry loop.

---

## 3. Path B — Dev run from source (no Docker)

### Step 1 — Toolchain

Go **1.25.0+** (`mothership/go.mod:3`). No C toolchain — `CGO_ENABLED=0` everywhere; SQLite
is pure Go (`modernc.org/sqlite`). Run Go commands from `mothership/`, not the repo root
(the root `go.work` has `use ./mothership` only; `go test ./...` from the root does not
resolve the same tree).

### Step 2 — Override the data directory (mandatory outside containers)

The default is `/data` (`config.go:119`). Without root, database open fails:

```
[FATAL] Failed to open main database: …      # main.go:759
```

(the mkdir failure itself only logs `[WARN] Failed to create data dir`, `main.go:895`;
CSI recording then degrades to a WARN at `main.go:899` — it is the DB open that kills the
process). Pick a writable directory:

```bash
export SPAXEL_DATA_DIR="$PWD/devdata"
mkdir -p "$SPAXEL_DATA_DIR"
```

### Step 3 — Run

```bash
cd mothership
go run ./cmd/mothership
```

Without `-tags=embed` the dashboard is served from disk, resolved by `findDashboardDir()`
(`main.go:291`): `./dashboard`, `./../dashboard`, `/app/dashboard` (first that exists), then
a repo-root-relative, CWD-independent fallback. So this works from `mothership/` **or** the
repo root, and frontend edits show up on refresh without a rebuild. Build-tagged variant,
if you want the embedded path:

```bash
cd mothership
mkdir -p cmd/mothership/dashboard && cp -r ../dashboard/. cmd/mothership/dashboard/
CGO_ENABLED=0 go build -tags=embed -o spaxel ./cmd/mothership   # as Dockerfile:56-61
./spaxel
```

Access: `http://localhost:8080/`. Set `SPAXEL_BIND_ADDR` to change it.

---

## 4. Path C — Prebuilt binary

The container binary is a distroless-style static Go binary (`CGO_ENABLED=0`,
`Dockerfile:56`; runtime `gcr.io/distroless/static-debian12:nonroot`, `Dockerfile:73`;
`ENTRYPOINT ["/spaxel"]`, `Dockerfile:99`). A binary built the same way runs anywhere:

```bash
SPAXEL_DATA_DIR=/var/lib/spaxel ./spaxel          # /firmware seed dir optional
```

Inside the committed container the firmware comes from `/firmware/spaxel-firmware.bin`
(copied in stage 3); outside it, point `SPAXEL_SEED_FIRMWARE_DIR` at a directory
containing the `.bin` files, or upload firmware later via `POST /api/firmware/upload`.

---

## 5. What a healthy boot looks like

Expected log sequence (format from `internal/startup/startup.go:38,41,74`; lines cited at
HEAD):

```
[INFO] Spaxel mothership v0.2.161 starting                     # main.go:715
[INFO] Current working directory: /app                         # main.go:722
[PHASE 1/7 — …] … [PHASE n/7 OK] (NNms)                        # db.OpenDB phases 1–4
[INFO] Main database at /data/spaxel.db                        # main.go:764
[INFO] Fleet registry at /data/fleet.db                        # main.go:949
[PHASE 5/7] Subsystem <name> started (NNms)                    # one per subsystem
[PHASE 6/7 — HTTP + mDNS]                                      # main.go:4968
[INFO] mDNS advertising spaxel._spaxel._tcp.local:8080         # main.go:4999
[INFO] HTTP server listening on 0.0.0.0:8080                   # main.go:5067
```

Data-dir files created on first boot (all normal): `spaxel.db`, `fleet.db`, `ble.db`,
`zones.db`, `analytics.db`, `anomaly.db`, `automation.db`, `notifications.db`,
`csi_replay.bin`, `csi/`, `firmware/`. If a firmware seed happened you will also see
`[INFO] firmware seed: installed <name>.bin`.

`docker compose ps` shows `healthy` only if the healthcheck can run — see §8 issue 7.

---

## 6. Verification checklist

Run these after start. `HOST` is the machine running the mothership.

```bash
# 1. Process health — the one endpoint that must work
curl -sf http://$HOST:8080/healthz | jq .
# → {"status":"ok","uptime_s":N,"version":"0.2.161","nodes_online":N,
#    "db":"ok","shedding_level":0}          (internal/health/health.go:50-56)
# HTTP 200 = ok; 503 + "reason" = degraded (health.go:66-69)

# 2. Dashboard HTML served
curl -sf http://$HOST:8080/ | head -5          # index.html DOCTYPE, not a 404

# 3. Live WebSocket feed upgrades (10 Hz blob/event push)
curl -s -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" \
     -H "Sec-WebSocket-Version: 13" \
     http://$HOST:8080/ws/dashboard -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ=="  # RFC 6455 sample nonce, gitleaks:allow
# → HTTP/1.1 101 Switching Protocols  (first frame is a "snapshot" message)

# 4. Node endpoint exists (rejects unauthenticated, which proves the route is live)
curl -s -o /dev/null -w '%{http_code}\n' http://$HOST:8080/ws/node    # non-101

# 5. mDNS advertisement (from another LAN host)
avahi-browse -r _spaxel._tcp                   # or: dns-sd -B _spaxel._tcp
```

In the browser: open `http://$HOST:8080/` — the status banner should show **Connected**
(not "Connecting..."), and `/live`, `/simple`, `/fleet`, `/setup`, `/ambient` all resolve.
No `Dashboard directory not found` warnings in the log (that warning means the dev-mode
asset search failed — §8 issue 5).

Nothing in this checklist requires a node to be attached. `nodes_online: 0` is valid —
the health checker explicitly does not degrade on zero nodes after warmup
(`health.go:134-135`).

---

## 7. First-run behaviour

1. **No PIN prompt.** Despite what `docs/plan/plan.md` describes, opening the dashboard at
   HEAD does **not** enter a PIN setup flow — the PIN middleware is unmounted (§0 fact 3).
   `/api/auth/*` endpoints exist and work (login/logout/change-pin, `spaxel_session`
   cookie), but nothing gates the dashboard or the API on them.
2. **Install secret** auto-generated and persisted to the DB on first run; override once at
   first boot with `SPAXEL_INSTALL_SECRET` (`config.go:218`).
3. **Firmware seed** copies `/firmware/*.bin` → `<data>/firmware/` (§2 step 4).
4. **WiFi seeding (ADR-005):** `SPAXEL_WIFI_SSID` + `SPAXEL_WIFI_PASSWORD` seed the DB
   **once, first boot only** (`seedWiFiCredentialsIfFirstBoot`, `main.go:661-695`); the DB
   setting is authoritative afterwards. Both must be set — one alone logs
   `[CONFIG] … must both be set … skipping seed`.

### 7.1 Security note (read before exposing the port)

With the compose file as committed — Traefik labels disabled by default
(`docker-compose.yml:86`, `TRAEFIK_ENABLE:-false`) — **anything that can reach TCP 8080
can read the dashboard and call the API**. Only node-facing routes are enforced:
`/ws/node` and `/firmware/*` require the `X-Spaxel-Token` header
(`main.go:4837`, `main.go:4857`). Put an authenticating reverse proxy in front, or bind to
loopback (`SPAXEL_BIND_ADDR=127.0.0.1:8080`), or keep the port LAN-private.

---

## 8. Troubleshooting

| # | Symptom | Cause | Fix |
|---|---|---|---|
| 1 | `[FATAL] HTTP server error: listen tcp …: bind: address already in use` | Port 8080 taken (host networking → no remap) | Free the port, or `SPAXEL_BIND_ADDR=0.0.0.0:<other>` in the compose `environment:` block |
| 2 | `[FATAL] Configuration validation failed:` + exit 1 | Malformed env — classically `SPAXEL_ADVERTISED_BASE_URL` with a wildcard/non-HTTP host (`config.go:94-101`) | Fix the variable; the failing lines are printed one per `[FATAL]` line (`main.go:708`) |
| 3 | `[FATAL] Failed to open main database: …` | Data dir missing/unwritable (dev run with default `/data`) | `SPAXEL_DATA_DIR` to a writable path (§3 step 2) |
| 4 | `docker compose up --build` dies in `firmware-fetcher` with curl 404 | `VERSION=dev` (compose default) has no GitHub release | `VERSION="$(cat VERSION)" docker compose up -d --build`, or use the prebuilt image (§2 steps 2-3) |
| 5 | Dashboard 404 / `[WARN] Dashboard directory not found` | Dev build can't find assets; embedded builds never hit this | Run from repo root or `mothership/` (both resolve via `main.go:295`), or set `SPAXEL_STATIC_DIR` (`config.go:122`) |
| 6 | No mDNS advertisement; nodes can't auto-discover | Docker bridge networking blocks multicast; or binding warning `[WARN] Could not bind mDNS to the node-reachable address …` (`main.go:4975`) | `network_mode: host`, or accept the fallback: `SPAXEL_MDNS_ENABLED=false` + cached/manual node IP |
| 7 | Container perpetually `unhealthy` while `/healthz` is fine from outside | Compose healthcheck runs `wget` (`docker-compose.yml:68`) — **no wget exists in the distroless runtime image** | Probe from outside the container (`curl http://$HOST:8080/healthz`); this healthcheck cannot execute as committed |
| 8 | Banner stuck on "Connecting..." | `/ws/dashboard` upgrade failing — process down, firewall, or a proxy killing long-lived WebSockets | Check `/healthz`; behind a proxy, extend read timeout (compose `:91-93` sets 3600 s) |
| 9 | OTA / onboarding has no firmware | `SPAXEL_FIRMWARE_DIR` set — **read by no code** (silent no-op); or empty `./firmware/` shadowing the baked payload | Real variable is `SPAXEL_SEED_FIRMWARE_DIR` (`config.go:126`); populate the bind mount (§2 step 4) |
| 10 | OTA trigger refuses to send | `SPAXEL_ADVERTISED_BASE_URL` unset and derivation failed → URL empty by design, `[WARN] OTA disabled: …` (ADR-004: fail loud instead of a guaranteed-failure `{"ok":true}`) | Set it to an address routable *from the nodes* — not `0.0.0.0`, not the bind address |
| 11 | Writes to the API return 403 | `SPAXEL_DEMO_MODE=true` rejects POST/PUT/PATCH/DELETE globally (`internal/auth/handler.go:730-748`) | Unset it; it is for public demos |
| 12 | Node connects then is dropped; `[WARN] … rolled back …` after a *successful* update | Mothership/firmware version namespaces disagree (firmware self-report vs filename-derived expectation) | Align both on the repo `VERSION` (ADR-004 decision 3); don't chase the rollback badge first |

---

## 9. Stopping and restarting

```bash
docker compose stop      # SIGTERM → 30s graceful drain (compose allows 35s, :60)
docker compose up -d     # restart — data persists in the spaxel-data volume
```

Graceful shutdown flushes baselines and the CSI buffer, closes node connections (nodes
reconnect on their own), and checkpoints SQLite. Restart is idempotent: no re-setup, no
re-seed of settings already in the DB (WiFi seeding is first-boot-only, §7 item 4), and a
re-pulled image re-seeds firmware only when the baked binary's size changed
(`main.go:5777-5782`).

For a dev run: Ctrl-C (SIGTERM/SIGINT are handled, `main.go:746`). Nothing else needs cleanup —
delete `SPAXEL_DATA_DIR` to start completely fresh.

---

## 10. Cross-references

- Dependency inventory, full 28-variable table, auth ground truth: `MOTHERSHIP_DASHBOARD_DEPENDENCIES.md`
- Startup sequence internals, serving modes, env surface with config.go line cites: `MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md`
- Asset locations and the embed mechanism: `MOTHERSHIP_DASHBOARD_LOCATIONS.md`
- Advertised base URL / OTA: `docs/plan/plan.md` ADR-004 · first-boot WiFi seeding: ADR-005 · node token on `/firmware`: ADR-006
- Two documented deviations from `docs/plan/plan.md` found while writing this: the compose file builds locally instead of pulling `ghcr.io/spaxel/spaxel` (§2 step 2), and the dashboard PIN gate is not mounted (§7 / §7.1)
