# Spaxel

**WiFi CSI-based indoor positioning for self-hosted homes.**

Spaxel detects and localizes people in a home using WiFi Channel State Information — no cameras, no microphones, no cloud. A single Docker container (the "mothership") runs on a home server and manages a fleet of ESP32-S3 nodes that capture and stream CSI over WiFi. The mothership fuses CSI from all links to detect presence, follow motion, and — with enough nodes — estimate 2D/3D position, rendered on a Three.js floor-plan dashboard.

Everything runs locally on hardware you own. There is no cloud relay, no account, and no RF beyond the 2.4 GHz WiFi already in your home.

## What it can realistically do

Based on physics and the research in [`docs/research/`](docs/research/) (see [`docs/research/06-accuracy-and-limits.md`](docs/research/06-accuracy-and-limits.md)). Each capability maps to a deterministic simulator acceptance scenario; the [acceptance map](docs/notes/localization-capability-acceptance-map.md) is the authority for scenario numbering, thresholds, and recorded results, and the status notes below summarize its latest recorded runs (§8 there).

- **Presence detection** — reliably, with 2+ nodes on opposite sides of a space. *Measured working* (AS-2; the deterministic runs record a scripted walker detected for 100 % of the post-warmup window)
- **Approximate 2D position** — ±1.0–1.5 m with 4+ nodes. *Measured within gate* (AS-8: median horizontal error 1.07–1.27 m across three recorded seed-42 runs, p90 ≈ 1.5 m, against the 1.5 m gate — 16–25 % of samples land inside 1.0 m; blobs stay within 2 m of ground truth). The original ±0.5–1.0 m estimate over-stated what the current fusion delivers, so the band was re-based on measured capability rather than left aspirational
- **Motion / trajectory tracking** — follows moving people. *Detection while moving measured working; trajectory accuracy measured below gate* (AS-2-ext: scripted-path walker tracked for 100 % of the run, but the nearest blob stays within 1.0 m of the ground-truth polyline for only 11.1 % of samples against the ≥ 80 % gate — median 1.14 m)
- **Rough person count** — distinguishes 1 vs. 2+ (degrades at 3+). *Partially met* (AS-9: 2+ people detected passes; a single person currently presents as several blobs — median 6 — so the "distinguishes 1" half does not yet hold)
- **Rough Z-axis** — ±1–2 m with mixed-height node placement (enables fall detection). *Measured working* (AS-3 fall chain including the bag-on-couch false-positive control; AS-3-ext: Z error within the 2 m gate — median 0.40 m standing, 1.00 m on the floor)
- **Stationary-person detection** — via breathing micro-motion (0.1–0.5 Hz). *Not yet validated* (AS-10: scenario assigned and its simulator fixture landed, acceptance test pending; two open breathing-detector defects mean the stationary state is not reachable end to end today)

**Not achievable** with 2.4 GHz CSI: sub-10 cm accuracy, skeletal pose, reliable 5+ person tracking — pinned by suite guards rather than assumed (AS-8's resolution floor, AS-3-ext's posture-only surface, AS-9's stability-only 5-walker run).

The status figures are snapshots of recorded simulator runs at a fixed seed, not guarantees for arbitrary rooms; the acceptance map carries the current numbers and updates whenever a scenario re-measures.

### Privacy by design

Spaxel is CSI-only — it never captures camera images or audio. Detection is local: data stays on the mothership, there is no cloud relay or remote access, and no user accounts are required (a single PIN protects the dashboard). See the *Non-Goals* section of [`docs/plan/plan.md`](docs/plan/plan.md).

## Repository layout

Spaxel is a [Go workspace](go.work) of one module (`mothership/`), plus ESP32 firmware and a static frontend:

| Path | Description |
|------|-------------|
| [`mothership/`](mothership/) | Go backend — ingestion, signal pipeline, localizer, fleet manager, REST/WebSocket API, dashboard server (`github.com/spaxel/mothership`) |
| [`mothership/cmd/sim/`](mothership/cmd/sim/) | `spaxel-sim` — CSI/node simulator CLI for hardware-free development and integration tests |
| [`mothership/test/acceptance/`](mothership/test/acceptance/) | Acceptance suite (AS-1 … AS-9 implemented, AS-10 assigned — see the [acceptance map](docs/notes/localization-capability-acceptance-map.md); IO-1 … IO-11), driven by the simulator |
| [`mothership/tests/e2e/`](mothership/tests/e2e/) | Go e2e package (plus the opt-in `-tags io6_gate` release hard gate) |
| [`firmware/`](firmware/) | ESP-IDF (C) firmware for the ESP32-S3 node fleet |
| [`dashboard/`](dashboard/) | Vanilla JS + Three.js single-page UI (see [`dashboard/README.md`](dashboard/README.md)) |
| [`docs/`](docs/) | Plan, notes, and research (see [Documentation](#documentation) below) |
| [`Dockerfile`](Dockerfile), [`docker-compose.yml`](docker-compose.yml) | Single-container packaging |
| [`PROGRESS.md`](PROGRESS.md), [`VERSION`](VERSION) | Implementation status and current version |

## Quickstart

The mothership ships as a single container, built from this repository. The bundled [`docker-compose.yml`](docker-compose.yml) builds it from source and exposes one port (8080) — no Docker Hub account or authentication is needed.

```bash
git clone https://github.com/jedarden/spaxel.git
cd spaxel
docker compose up -d        # builds the mothership image, host networking
```

> **About the published image:** CI also pushes the image to `ronaldraygun/spaxel` on Docker Hub, but that repository is **private** — it serves the maintainer's own deployments and is not a distribution channel. An unauthenticated `docker pull ronaldraygun/spaxel` returns a 404: an auth wall, not a missing repository (`docker manifest inspect` distinguishes the two). Building from source, as above, is the only supported install path.

Then open `http://<server-ip>:8080`, set a dashboard PIN, and use **Add Node** (Chrome/Edge Web Serial) to provision an ESP32-S3 over USB. The node discovers the mothership via mDNS and begins streaming CSI — zero manual IP configuration.

> `network_mode: host` is required for mDNS multicast to reach ESP32 nodes on your LAN. If host networking isn't available, set `SPAXEL_MDNS_ENABLED=false` and provision nodes with a manual mothership IP (see [`docs/notes/mdns-override.md`](docs/notes/mdns-override.md)).

### Key environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` | Listen address (set to `127.0.0.1:8080` behind a local reverse proxy) |
| `SPAXEL_DATA_DIR` | `/data` | Persistent storage: SQLite, baselines, floor plans, CSI replay buffer, firmware |
| `SPAXEL_MQTT_BROKER` | *(unset)* | Optional MQTT broker URL for Home Assistant integration (e.g. `mqtt://homeassistant.local:1883`) |
| `TZ` | `UTC` | Timezone for diurnal baselines, briefings, and quiet hours (IANA name) |
| `SPAXEL_MDNS_ENABLED` | `true` | Disable when not using host networking |
| `SPAXEL_DEMO_MODE` | `false` | Read-only demo dashboard: mutating API calls return `403`, everything else is browsable with no PIN |

### Demo mode

Set `SPAXEL_DEMO_MODE=true` to run Spaxel as a read-only showcase — handy for screenshots, kiosk displays, or letting someone explore the dashboard on a live deployment. Every mutating API request (`POST`/`PUT`/`PATCH`/`DELETE`) is rejected with `403` before it reaches a handler, while all read endpoints and dashboard pages stay open with no PIN prompt. Because first-run PIN setup is itself a `POST`, set the PIN *before* enabling demo mode. `SPAXEL_DEMO_MAX_DASHBOARD_CLIENTS` (default 5) caps concurrent dashboard WebSocket viewers while the mode is on.

The full list is in the *Deployment* section of [`docs/plan/plan.md`](docs/plan/plan.md).

### WiFi Configuration

**Mothership-level fleet-wide configuration:** Spaxel uses a fleet-wide WiFi credential model — you configure the network credentials **once** on the mothership, and all nodes automatically join that network during provisioning.

**Configuration methods:**

1. **Dashboard UI (recommended):** Settings > Network panel — enter SSID/password once, all nodes auto-join
2. **REST API:** `PUT /api/settings/network` with `{"wifi_ssid":"...","wifi_password":"..."}`
3. **Headless first boot:** set `SPAXEL_WIFI_SSID` and `SPAXEL_WIFI_PASSWORD` on the mothership; they seed the fleet setting once

**Credential precedence:**
1. Database settings (`network_wifi_ssid`/`network_wifi_password` from Settings > Network)
2. First-boot environment seed (`SPAXEL_WIFI_SSID`/`SPAXEL_WIFI_PASSWORD`)
3. An explicit request-body override is retained only for direct API callers that intentionally provision a different network; the onboarding wizard never asks for or sends per-device credentials

**Important notes:**
- Environment variables seed an empty mothership database only; after the first boot, the stored Settings > Network value is authoritative
- WiFi password is **write-only** — never returned by the API for security
- See [`docs/deployment/wifi-configuration.md`](docs/deployment/wifi-configuration.md) for deployment examples and migration guide

## Building & developing

```bash
# Mothership backend
cd mothership && go test ./... && go vet ./...

# CSI / node simulator
go build -o /tmp/spaxel-sim ./cmd/sim

# Acceptance suite (no ESP32 needed; needs a built mothership + spaxel-sim in PATH)
SPAXEL_INTEGRATION_TEST=1 go test ./test/acceptance/

# Dashboard unit + accessibility tests (CI quality gate)
cd dashboard && npm test && npm run test:a11y
```

Firmware is built with ESP-IDF 5.2.x — see the *Firmware Build System* section of the plan.

## Documentation

- [`docs/plan/plan.md`](docs/plan/plan.md) — the complete design: architecture, components, schema, deployment, phases
- [`docs/notes/`](docs/notes/) — implementation notes (recovery mechanisms, mDNS override, simulation testing, UX)
- [`docs/notes/localization-capability-acceptance-map.md`](docs/notes/localization-capability-acceptance-map.md) — maps every capability claim above to its acceptance scenario; the scenario-numbering and threshold authority for the acceptance suite
- [`docs/notes/dashboard-pin-auth.md`](docs/notes/dashboard-pin-auth.md) — the dashboard PIN specification: setup, session, failed-login, and reset behavior, where enforcement actually happens per deployment, and the test map for dashboard, REST, and WebSocket access
- [`docs/research/`](docs/research/) — CSI fundamentals, physics, algorithms, accuracy limits, prior-art papers
- [`docs/ci-accessibility-integration.md`](docs/ci-accessibility-integration.md) — CI accessibility testing quality gate (WCAG 2.1 AA)
- [`docs/ci-benchmark-integration.md`](docs/ci-benchmark-integration.md) — CI timing benchmark quality gate (fusion loop)
- [`dashboard/README.md`](dashboard/README.md) — dashboard test setup (Jest, axe-core + Playwright)
- [`PROGRESS.md`](PROGRESS.md) — phase-by-phase implementation status

### Error Handling Best Practices

The Spaxel firmware uses explicit error checking patterns to ensure reliable operation and prevent abort loops. Key principles:

**Never use `ESP_ERROR_CHECK` in application code** — It aborts the system on any error, creating restart loops for transient failures. Use explicit error checking instead:

```c
esp_err_t err = some_function();
if (err != ESP_OK) {
    ESP_LOGE(TAG, "Operation failed: %s", esp_err_to_name(err));
    return err;  // Let caller decide what to do
}
```

**Restart-safe guard pattern** — Check restart flags before WiFi operations to prevent race conditions:

```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping operation - restart imminent");
    return ESP_OK;  // Graceful skip, NOT an error
}
```

See [`docs/notes/adr-010-error-handling-patterns.md`](docs/notes/adr-010-error-handling-patterns.md) for the complete architecture decision record and [`docs/notes/error-handling-patterns.md`](docs/notes/error-handling-patterns.md) for detailed implementation guidance. The restart-safe pattern is documented inline in [`firmware/main/wifi.h`](firmware/main/wifi.h).

---

*Spaxel is self-hosted, CSI-only, and cloud-free by design.*
