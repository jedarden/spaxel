# Mothership Dashboard Startup Investigation

**Investigation Date:** 2026-08-28  
**Bead ID:** spaxel-c79cdd86  
**Status:** COMPLETE

---

## Executive Summary

The Mothership dashboard is a single-page web application served by the Go backend. It is NOT started as a separate process—the dashboard is an integral part of the Mothership Go application, served via embedded filesystem (production) or local filesystem (development).

---

## 1. Dashboard Code Location

### Primary Directory
```
/home/coding/spaxel/dashboard/
├── index.html          # Main dashboard entry point
├── live.html           # Live 3D view (Three.js)
├── simple.html         # Simplified mobile-first view
├── ambient.html        # Wall-mounted tablet mode
├── simulator.html      # Node simulator interface
├── setup.html          # Node onboarding wizard
├── fleet.html          # Fleet status and management
├── integrations.html   # Home Assistant/MQTT settings
├── css/                # Stylesheets
├── js/                 # Frontend JavaScript
├── static/             # Icons, images
└── types/              # TypeScript definitions
```

### Backend Integration
- **Mothership entry point:** `/home/coding/spaxel/mothership/cmd/mothership/main.go`
- **Dashboard package:** `/home/coding/spaxel/mothership/internal/dashboard/`
- **Embed directive:** `/home/coding/spaxel/mothership/cmd/mothership/dashboard_embed.go`

---

## 2. Startup Method

### Architecture
The dashboard is **NOT a separate server**. It is served by the Mothership Go application:

```
┌─────────────────────────────────────┐
│  Mothership Go Application        │
│  (mothership/cmd/mothership)      │
│                                    │
│  ┌───────────────────────────┐   │
│  │ HTTP Server (port 8080)   │   │
│  │                           │   │
│  │ ┌─────────────────────┐   │   │
│  │ │ Dashboard Routes    │   │   │
│  │ │ - GET /*            │   │   │
│  │ │ - GET /ws/dashboard  │◄──┼──┼─── WebSocket for live updates
│  │ └─────────────────────┘   │   │
│  └───────────────────────────┘   │
└─────────────────────────────────────┘
         │
         ▼
    Browser connects to
    http://host:8080/
```

### Server Initialization Sequence

**Phase 6** of the 7-phase startup sequence (main.go lines 4936-5040):

1. **HTTP Server Creation** (line 5027-5032):
   ```go
   srv := &http.Server{
       Addr:         cfg.BindAddr,        // Default: 0.0.0.0:8080
       Handler:      r,                   // chi.Router with all routes
       ReadTimeout:  10 * time.Second,
       WriteTimeout: 30 * time.Second,
   }
   ```

2. **Dashboard Hub & Server** (line 1807-1808):
   ```go
   dashboardHub = dashboard.NewHub(cfg.MaxDashboardClients)
   dashboardSrv := dashboard.NewServer(dashboardHub)
   ```

3. **Dashboard WebSocket Registration** (line 4864):
   ```go
   r.HandleFunc("/ws/dashboard", dashboardSrv.HandleDashboardWS)
   ```

4. **Static File Serving** (line 4893-4929):
   - **Production:** Embedded filesystem via `//go:embed dashboard`
   - **Development:** Filesystem fallback searches multiple directories

5. **Server Start** (line 5034-5039):
   ```go
   go func() {
       log.Printf("[INFO] HTTP server listening on %s", cfg.BindAddr)
       if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
           log.Fatalf("[FATAL] HTTP server error: %v", err)
       }
   }()
   ```

---

## 3. Dashboard Serving Modes

### Production Build (Embedded)
- **File:** `mothership/cmd/mothership/dashboard_embed.go`
- **Build tag:** `//go:build embed`
- **Mechanism:** Dashboard files copied into `mothership/cmd/mothership/dashboard/` before compile
- **Embed directive:** `//go:embed dashboard`
- **Result:** Single binary with dashboard assets included

### Development Build (Filesystem)
- **Fallback paths searched in order:**
  1. `./dashboard` (from repo root)
  2. `./../dashboard` (from mothership/ subdirectory)
  3. `/app/dashboard` (container path)
- **Handler:** `dashboardStaticHandler()` using `http.ServeFile()`
- **Advantage:** Live edits without recompilation

---

## 4. Dependencies and Prerequisites

### Required

| Item | Requirement | Default | Configurable Via |
|------|-------------|----------|-------------------|
| **Port** | 8080 | `0.0.0.0:8080` | `SPAXEL_BIND_ADDR` env var |
| **Data Directory** | `/data` | `/data` | `SPAXEL_DATA_DIR` env var |
| **mDNS** | Multicast-enabled network | Required for node auto-discovery | `SPAXEL_MDNS_ENABLED=true` (default) |
| **Network Mode** | `host` (Docker) | Required for mDNS multicast | `network_mode: host` in docker-compose.yml |

### Optional

| Item | Purpose | Default |
|------|---------|---------|
| **MQTT Broker** | Home Assistant integration | Disabled (set `SPAXEL_MQTT_BROKER`) |
| **NTP Server** | Node time sync | `pool.ntp.org` |
| **Timezone** | Diurnal baselines | `TZ` env var (UTC) |
| **Traefik** | Reverse proxy + TLS | Optional labels in compose |
| **Resource Limits** | Fleet size scaling | 512m RAM, 2.0 CPUs |

### Docker-Specific Requirements

```yaml
# From docker-compose.yml
network_mode: host              # REQUIRED for mDNS
cap_add:
  - NET_BIND_SERVICE           # For local NTP server (UDP 123)
volumes:
  - spaxel-data:/data          # SQLite, CSI buffer, floor plans
  - ./firmware:/firmware:ro     # OTA firmware binaries
```

---

## 5. Commands to Start

### Development Mode (Local Go Build)

**Option 1: Run directly**
```bash
cd /home/coding/spaxel/mothership
go run cmd/mothership/*.go
```

**Option 2: Build and run**
```bash
cd /home/coding/spaxel/mothership
go build -o mothership cmd/mothership/*.go
./mothership
```

**Access:** `http://localhost:8080/`

### Docker Mode (Production)

**Using docker-compose**
```bash
cd /home/coding/spaxel
docker-compose up -d
```

**Using docker run**
```bash
docker run -d --name spaxel \
  --network host \
  -v spaxel-data:/data \
  -v ./firmware:/firmware:ro \
  -e TZ=America/New_York \
  ghcr.io/spaxel/spaxel:latest
```

### Pre-built Binary

If you have the compiled binary (e.g., from CI):
```bash
cd /home/coding/spaxel/mothership
./mothership
```

---

## 6. Access Points

| Endpoint | Purpose | Method |
|----------|---------|--------|
| `http://host:8080/` | Dashboard home | GET |
| `http://host:8080/live` | Live 3D view | GET |
| `http://host:8080/simple` | Simple mode | GET |
| `http://host:8080/fleet` | Fleet status | GET |
| `http://host:8080/setup` | Node onboarding | GET |
| `http://host:8080/ambient` | Ambient display | GET |
| `http://host:8080/ws/dashboard` | WebSocket for live updates | WebSocket upgrade |
| `http://host:8080/healthz` | Health check | GET |

---

## 7. Startup Sequence Details

The Mothership follows a **7-phase startup sequence** (main.go line 702+):

### Phase 1/7 — Data Directory
- Verify `/data` is writable
- Acquire file lock to prevent duplicate instances
- **Timeout:** 5 seconds

### Phase 2/7 — SQLite Database
- Open `spaxel.db` with WAL mode
- Run schema migrations
- **Timeout:** 10 seconds

### Phase 3/7 — Schema Migration
- Apply pending migrations in order
- **Timeout:** 10 seconds

### Phase 4/7 — Config & Secrets
- Load environment variables
- Validate configuration
- Generate/load `install_secret`
- **Timeout:** 5 seconds

### Phase 5/7 — Subsystems Initialization
- Start ingestion server
- Initialize signal processing pipeline
- Create fleet manager
- **Create dashboard hub and server** ← **Dashboard initialized here**
- Wire up all components (fusion, BLE, zones, etc.)
- **Timeout:** 60 seconds

### Phase 6/7 — HTTP Server & mDNS
- **Create HTTP server** on configured port
- **Register dashboard routes** (static files + WebSocket)
- Start mDNS advertisement (if enabled)
- **Start HTTP server** (goroutine)
- **Timeout:** 30 seconds

### Phase 7/7 — Health Check & Readiness
- Verify `/healthz` endpoint responds
- Write ready marker file
- Log "All 7 phases completed"
- **Timeout:** 10 seconds

**Total startup timeout:** 30 seconds (shared across all phases)

---

## 8. Network Configuration

### Port Binding
- **Default:** `0.0.0.0:8080` (all interfaces)
- **Config:** `SPAXEL_BIND_ADDR` environment variable
- **Docker note:** With `network_mode: host`, port 8080 is directly exposed on the host

### mDNS Advertisement
- **Service name:** `_spaxel._tcp.local`
- **TXT records:** `version=1`, `ws=/ws/node`, `dashboard=/ws/dashboard`
- **Purpose:** ESP32 nodes auto-discover mothership without manual IP configuration
- **Requirement:** Host networking (Docker) or multicast-capable network (native)

### WebSocket Endpoints
1. **Node → Mothership:** `ws://host:8080/ws/node`
   - Binary CSI frames upstream
   - JSON config/commands downstream

2. **Dashboard → Mothership:** `ws://host:8080/ws/dashboard`
   - 10 Hz blob updates
   - Event notifications
   - System state changes

---

## 9. Verification Checklist

To verify the dashboard is running correctly:

- [ ] HTTP server starts on port 8080
- [ ] `/healthz` returns `{"status":"ok"}` with HTTP 200
- [ ] Dashboard loads at `http://host:8080/`
- [ ] WebSocket connects at `/ws/dashboard`
- [ ] Status banner shows "Connected" (not "Connecting...")
- [ ] No "Dashboard directory not found" warnings in logs
- [ ] mDNS advertising `_spaxel._tcp.local` (check with `avahi-browse _spaxel._tcp.local`)

---

## 10. Common Issues and Solutions

### Issue: Dashboard shows "Connecting..." indefinitely
**Cause:** WebSocket connection failed  
**Check:** 
- `/healthz` endpoint
- Firewall blocking port 8080
- Mothership crashed (check logs)

### Issue: "Dashboard directory not found" warning
**Cause:** Development mode cannot find dashboard/ directory  
**Solution:** Run from repo root or ensure dashboard/ exists in searched paths

### Issue: mDNS not advertising
**Cause:** Docker bridge networking blocks multicast  
**Solution:** Use `network_mode: host` in docker-compose.yml

### Issue: Port 8080 already in use
**Cause:** Another process bound to port 8080  
**Solution:** Stop conflicting process or set `SPAXEL_BIND_ADDR` to different port

---

## 11. File Paths Reference

### Key Files

| Purpose | Path |
|---------|------|
| **Dashboard assets** | `/home/coding/spaxel/dashboard/` |
| **Main entry point** | `/home/coding/spaxel/mothership/cmd/mothership/main.go` |
| **Dashboard embed** | `/home/coding/spaxel/mothership/cmd/mothership/dashboard_embed.go` |
| **Dashboard hub** | `/home/coding/spaxel/mothership/internal/dashboard/` |
| **HTTP routes** | `/home/coding/spaxel/mothership/cmd/mothership/main.go:4936-5040` |
| **Dockerfile** | `/home/coding/spaxel/Dockerfile` |
| **Compose file** | `/home/coding/spaxel/docker-compose.yml` |

### Configuration Files

| File | Purpose |
|------|---------|
| `docker-compose.yml` | Production deployment configuration |
| `Dockerfile` | Multi-stage build (firmware + Go binary) |
| `.env` (optional) | Environment variables for configuration |
| `/data/spaxel.db` | SQLite database (runtime, not in repo) |

---

## Conclusion

The Mothership dashboard is **not a standalone service**. It is served as part of the Go Mothership application, with static files either embedded in the binary (production) or served from the filesystem (development). The dashboard is accessible immediately upon HTTP server startup in Phase 6 of the Mothership's 7-phase initialization sequence.

**Key takeaways:**
1. Start the Mothership Go application → dashboard starts automatically
2. Default port is 8080 (configurable via `SPAXEL_BIND_ADDR`)
3. Requires host networking for mDNS (node auto-discovery)
4. No separate dashboard startup command or process
5. WebSocket endpoint `/ws/dashboard` provides real-time updates

---

**Next Steps (if applicable):**
- Test dashboard startup with: `./mothership` or `docker-compose up`
- Verify `/healthz` endpoint responds
- Check browser console for WebSocket connection
- Review logs for "Dashboard directory not found" warnings
