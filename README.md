# Spaxel

**WiFi CSI-based indoor positioning for self-hosted homes.**

Spaxel detects and localizes people in a home using WiFi Channel State Information — no cameras, no microphones, no cloud. A single Docker container (the "mothership") runs on a home server and manages a fleet of ESP32-S3 nodes that capture and stream CSI over WiFi. The mothership fuses CSI from all links to detect presence, follow motion, and — with enough nodes — estimate 2D/3D position, rendered on a Three.js floor-plan dashboard.

Everything runs locally on hardware you own. There is no cloud relay, no account, and no RF beyond the 2.4 GHz WiFi already in your home.

## What it can realistically do

Based on physics and the research in [`docs/research/`](docs/research/) (see [`docs/research/06-accuracy-and-limits.md`](docs/research/06-accuracy-and-limits.md)):

- **Presence detection** — reliably, with 2+ nodes on opposite sides of a space
- **Approximate 2D position** — ±0.5–1.0 m with 4+ nodes
- **Motion / trajectory tracking** — follows moving people
- **Rough person count** — distinguishes 1 vs. 2+ (degrades at 3+)
- **Rough Z-axis** — ±1–2 m with mixed-height node placement (enables fall detection)
- **Stationary-person detection** — via breathing micro-motion (0.1–0.5 Hz)

**Not achievable** with 2.4 GHz CSI: sub-10 cm accuracy, skeletal pose, reliable 5+ person tracking.

### Privacy by design

Spaxel is CSI-only — it never captures camera images or audio. Detection is local: data stays on the mothership, there is no cloud relay or remote access, and no user accounts are required (a single PIN protects the dashboard). See the *Non-Goals* section of [`docs/plan/plan.md`](docs/plan/plan.md).

## Repository layout

Spaxel is a [Go workspace](go.work) of three modules, plus ESP32 firmware and a static frontend:

| Path | Description |
|------|-------------|
| [`mothership/`](mothership/) | Go backend — ingestion, signal pipeline, localizer, fleet manager, REST/WebSocket API, dashboard server (`github.com/spaxel/mothership`) |
| [`cmd/sim/`](cmd/sim/) | `spaxel-sim` — CSI/node simulator CLI for hardware-free development and integration tests (`github.com/spaxel/sim`) |
| [`test/acceptance/`](test/acceptance/) | Acceptance-test module (AS-1 … AS-7), driven by the simulator |
| [`firmware/`](firmware/) | ESP-IDF (C) firmware for the ESP32-S3 node fleet |
| [`dashboard/`](dashboard/) | Vanilla JS + Three.js single-page UI (see [`dashboard/README.md`](dashboard/README.md)) |
| [`docs/`](docs/) | Plan, notes, and research (see [Documentation](#documentation) below) |
| [`Dockerfile`](Dockerfile), [`docker-compose.yml`](docker-compose.yml) | Single-container packaging |
| [`PROGRESS.md`](PROGRESS.md), [`VERSION`](VERSION) | Implementation status and current version |

## Quickstart

The mothership ships as a single container, published as `ronaldraygun/spaxel`. The bundled [`docker-compose.yml`](docker-compose.yml) builds from source by default and exposes one port (8080).

```bash
git clone https://github.com/jedarden/spaxel.git
cd spaxel
docker compose up -d        # builds the mothership image, host networking
# …or skip the build and use the published image:
#   docker pull ronaldraygun/spaxel
```

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

The full list is in the *Deployment* section of [`docs/plan/plan.md`](docs/plan/plan.md).

## Building & developing

```bash
# Mothership backend
cd mothership && go test ./... && go vet ./...

# CSI / node simulator
go build -o spaxel-sim ./cmd/sim

# Hardware-free acceptance suite (no ESP32 needed)
cd test/acceptance && go test ./...

# Dashboard unit + accessibility tests
cd dashboard && npm test && npm run test:a11y
```

Firmware is built with ESP-IDF 5.2.x — see the *Firmware Build System* section of the plan.

## Documentation

- [`docs/plan/plan.md`](docs/plan/plan.md) — the complete design: architecture, components, schema, deployment, phases
- [`docs/notes/`](docs/notes/) — implementation notes (recovery mechanisms, mDNS override, simulation testing, UX)
- [`docs/research/`](docs/research/) — CSI fundamentals, physics, algorithms, accuracy limits, prior-art papers
- [`dashboard/README.md`](dashboard/README.md) — dashboard test setup (Jest, axe-core + Playwright)
- [`PROGRESS.md`](PROGRESS.md) — phase-by-phase implementation status

---

*Spaxel is self-hosted, CSI-only, and cloud-free by design.*
