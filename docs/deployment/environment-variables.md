# Spaxel Environment Variables Reference

**Last updated:** 2026-08-11  
**Purpose:** Complete reference for Spaxel environment variables with implementation status

## Overview

Spaxel mothership configuration is primarily done through the dashboard Settings panels or REST API. Environment variables also support headless deployments, including first-boot seeding of the fleet WiFi configuration.

### WiFi Environment Variables

`SPAXEL_WIFI_SSID` and `SPAXEL_WIFI_PASSWORD` are read at startup. When the
database has no network setting yet, both values seed the mothership's
`network_wifi_ssid` and `network_wifi_password` settings. The database remains
authoritative on later boots, so changing the environment does not silently
overwrite a value changed in Settings > Network.

For interactive changes, use:
- Dashboard UI: Settings > Network panel
- REST API: `PUT /api/settings/network`

See [`wifi-configuration.md`](wifi-configuration.md) for complete WiFi configuration guide.

---

## Environment Variables by Category

### Core Mothership Settings

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` | HTTP/WebSocket listen address | ✅ Implemented |
| `SPAXEL_DATA_DIR` | `/data` | Persistent storage path (SQLite, baselines, CSI replay, firmware) | ✅ Implemented |
| `TZ` | `UTC` | Timezone for diurnal baselines, morning briefings, quiet hours | ✅ Implemented |
| `SPAXEL_DEMO_MODE` | `false` | Demo mode — read-only showcase dashboard: mutating REST requests (`POST`/`PUT`/`PATCH`/`DELETE`) are rejected with `403`, all `GET` endpoints and dashboard pages stay open with no PIN. Set the PIN before enabling (setup is itself a `POST`). Accepts `true`/`1`/`false`/`0` | ✅ Implemented |

### Networking & Discovery

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_MDNS_ENABLED` | `true` | Enable/disable mDNS advertisement for node discovery | ✅ Implemented |
| `SPAXEL_MDNS_NAME` | `spaxel` | mDNS service name advertised | ✅ Implemented |
| `SPAXEL_NTP_SERVER` | `pool.ntp.org` | NTP server for node clock synchronization (embedded in provisioning payload) | ✅ Implemented |

### WiFi Configuration

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_WIFI_SSID` | *(unset)* | First-boot seed for the fleet WiFi SSID | ✅ Implemented |
| `SPAXEL_WIFI_PASSWORD` | *(unset)* | First-boot seed for the fleet WiFi password | ✅ Implemented |

Both variables must be set together for seeding. The onboarding wizard never
collects per-device WiFi credentials; it asks the mothership for the generated
payload instead.

### MQTT Integration (Optional)

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_MQTT_BROKER` | *(unset)* | MQTT broker URL (e.g., `mqtt://homeassistant:1883`) | ✅ Implemented |
| `SPAXEL_MQTT_USERNAME` | *(none)* | MQTT username | ✅ Implemented |
| `SPAXEL_MQTT_PASSWORD` | *(none)* | MQTT password | ✅ Implemented |
| `SPAXEL_MQTT_PREFIX` | `spaxel` | MQTT topic prefix | ✅ Implemented |
| `SPAXEL_MQTT_CLIENT_ID` | `spaxel-<install_id>` | MQTT client ID override | ✅ Implemented |

### CSI Replay Buffer

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_REPLAY_MAX_MB` | `360` | Maximum CSI replay buffer size in MB | ✅ Implemented |
| `SPAXEL_REPLAY_RETAIN_H` | `48` | CSI replay retention in hours | ✅ Implemented |

### Performance Tuning

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_FUSION_RATE_HZ` | `10` | Fusion loop rate (Hz) — affects detection latency | ✅ Implemented |
| `SPAXEL_GRID_CELL_M` | `0.2` | Fresnel zone grid cell size (meters) | ✅ Implemented |
| `SPAXEL_NODE_STALE_S` | `15` | Seconds before node marked STALE if no health received | ✅ Implemented |
| `SPAXEL_MAX_DASHBOARD_CLIENTS` | `10` | Maximum concurrent dashboard WebSocket clients | ✅ Implemented |
| `SPAXEL_DEMO_MAX_DASHBOARD_CLIENTS` | `5` | Demo-mode dashboard WebSocket client cap — overrides `SPAXEL_MAX_DASHBOARD_CLIENTS` when `SPAXEL_DEMO_MODE=true` | ✅ Implemented |

### OTA & Updates

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_ADVERTISED_BASE_URL` | *(auto-derived)* | Base URL for OTA firmware downloads (auto-derived from bind address) | ✅ Implemented |
| `SPAXEL_FIRMWARE_DIR` | `/firmware` | Firmware binary directory for OTA | ✅ Implemented |

### Debugging & Logging

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) | ✅ Implemented |
| `SPAXEL_SKIP_MIGRATIONS` | `false` | Skip automatic database migrations on startup (advanced) | ✅ Implemented |

### Auto-Update Settings

| Variable | Default | Purpose | Implementation Status |
|----------|---------|---------|------------------------|
| `SPAXEL_AUTO_UPDATE_ENABLED` | `false` | Enable automatic fleet-wide firmware updates | ✅ Implemented |
| `SPAXEL_AUTO_UPDATE_CANARY_DURATION_H` | `10` | Canary observation duration (hours) before fleet rollout | ✅ Implemented |
| `SPAXEL_AUTO_UPDATE_QUIET_WINDOW_START` | `02:00` | Quiet window start time (HH:MM format) | ✅ Implemented |
| `SPAXEL_AUTO_UPDATE_QUIET_WINDOW_END` | `05:00` | Quiet window end time (HH:MM format) | ✅ Implemented |

---

## Implementation Status Legend

| Status | Meaning |
|--------|---------|
| ✅ **Implemented** | Variable is read by code and affects behavior |
| ❌ **NOT Implemented** | Variable is documented but has no code support; setting it does nothing |
| ⚠️ **Partial** | Variable is implemented but with caveats or limited functionality |

---

## Environment Variable Loading

**Configuration file:** `mothership/internal/config/config.go`

**Loading mechanism:** Environment variables are read at startup using standard Go `os.Getenv()` calls. Variables are validated and typed before use.

**Persistence:** Changes to environment variables require container restart to take effect.

---

## WiFi Configuration: First-Boot Environment Seeding

### Design Intent (ADR-005)

The plan document (`docs/plan/plan.md` ADR-005) describes the intended behavior:

> "On first boot, if set and no `network` setting exists yet in the DB, they seed it once. After that the DB row is the source of truth; the env var is not re-read or re-applied"

### Current Reality

The first-boot seeding path is implemented:

1. `mothership/internal/config/config.go` reads both variables at startup.
2. `mothership/cmd/mothership/main.go` seeds both database settings when neither exists.
3. Existing settings are preserved, so later restarts do not overwrite operator changes.

### Why This Matters

- **Scripted deployments:** Can pre-configure WiFi via environment variables in compose files
- **Infrastructure as code:** Can inject credentials through deployment secrets (do not commit them to Git)
- **First-run automation:** Cannot auto-configure without dashboard interaction or API calls

### Working Alternatives

**For automated deployments, environment seeding or the API can be used:**

```bash
# After container starts, configure WiFi via API
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

Or in a startup script:

```bash
#!/bin/sh
# Wait for mothership to be ready
sleep 10

# Set PIN (first-run)
curl -X POST http://localhost:8080/api/auth/setup \
  -H "Content-Type: application/json" \
  -d '{"pin":"1234"}'

# Configure WiFi
curl -X PUT http://localhost:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

---

## Complete docker-compose.yml Example

```yaml
services:
  spaxel:
    image: ronaldraygun/spaxel:latest
    network_mode: host  # Required for mDNS
    volumes:
      - spaxel-data:/data
    environment:
      # Core settings
      - TZ=America/New_York
      - SPAXEL_DATA_DIR=/data
      - SPAXEL_BIND_ADDR=0.0.0.0:8080
      
      # Networking
      - SPAXEL_MDNS_ENABLED=true
      - SPAXEL_MDNS_NAME=spaxel
      
      # NTP for nodes
      - SPAXEL_NTP_SERVER=pool.ntp.org
      
      # Performance tuning
      - SPAXEL_FUSION_RATE_HZ=10
      - SPAXEL_GRID_CELL_M=0.2
      
      # Replay buffer
      - SPAXEL_REPLAY_MAX_MB=360
      - SPAXEL_REPLAY_RETAIN_H=48
      
      # MQTT (optional)
      # - SPAXEL_MQTT_BROKER=mqtt://homeassistant:1883
      # - SPAXEL_MQTT_USERNAME=spaxel
      # - SPAXEL_MQTT_PASSWORD=secret
      
      # Logging
      - SPAXEL_LOG_LEVEL=info
      
      # Optional first-boot seed for the mothership fleet WiFi settings
      - SPAXEL_WIFI_SSID=MyNetwork
      - SPAXEL_WIFI_PASSWORD=MyPassword
      # Alternatively, configure the network in dashboard Settings > Network
    restart: unless-stopped

volumes:
  spaxel-data:
```

**Post-deployment configuration required:**

```bash
# 1. Open dashboard at http://<server-ip>:8080
# 2. Complete first-run PIN setup
# 3. Navigate to Settings > Network
# 4. Enter WiFi SSID and password
# 5. All subsequent nodes will auto-join this network
```

Or via API:

```bash
curl -X PUT http://<server-ip>:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

---

## Security Considerations for Environment Variables

### Variables That Should Never Be Committed to Git

- **`SPAXEL_INSTALL_SECRET`** (if manually set) — Installation-wide secret
- **`SPAXEL_MQTT_PASSWORD`** — MQTT broker password
- **Any database credentials** (if using external PostgreSQL)

### Safe Variables for Git

- `SPAXEL_BIND_ADDR` — Binding address only
- `SPAXEL_DATA_DIR` — Path only
- `TZ` — Timezone name only
- `SPAXEL_MDNS_ENABLED` — Boolean flag
- `SPAXEL_LOG_LEVEL` — Log level name
- All tuning and performance variables

### Recommended Practice

1. **Use Kubernetes Secrets** for sensitive values in K8s deployments
2. **Use Docker Secrets** or **Swarm secrets** for sensitive values in Swarm
3. **Use .env files** (gitignored) for local development
4. **Never commit** `.env` files with real credentials

---

## Related Documentation

- [`wifi-configuration.md`](wifi-configuration.md) — Complete WiFi configuration guide with deployment examples
- [`../wifi-credential-provisioning-flow.md`](../wifi-credential-provisioning-flow.md) — Technical audit of WiFi credential implementation
- [`../plan/plan.md`](../plan/plan.md) — Architecture Decision Records, including ADR-005
- [`../../README.md`](../../README.md) — Project quickstart

---

## Quick Reference: Configuring WiFi (The Short Version)

**For WiFi configuration, use one of these methods:**

**Dashboard UI:**
1. Open `http://<server-ip>:8080`
2. Settings → Network
3. Enter SSID/password → Save

**API:**
```bash
curl -X PUT http://<server-ip>:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

**Verify:**
```bash
curl http://<server-ip>:8080/api/settings/network
```

That's it. All nodes provisioned afterward will automatically join this network.
