# Spaxel Implementation Progress

## Phase 1 — Foundation

Goal: Bare-minimum loop from ESP32 to browser. Zero-config with passive radar and mDNS from day one.

### Status: COMPLETE

| Item | Status | Notes |
|------|--------|-------|
| ESP32 firmware skeleton | **Done** | See iteration 2 below |
| Passive radar support | **Done** | BSSID filter in csi.c |
| BLE scanning | **Done** | Core 0 concurrent with WiFi |
| Mothership WebSocket ingestion | **Done** | See iteration 1 below |
| Dashboard skeleton | **Done** | See iteration 3 below |
| Docker packaging | **Done** | See iteration 4 below |

## Phase 2 — Signal Processing & Detection

Goal: Detect presence on a single link.

### Status

| Item | Status | Notes |
|------|--------|-------|
| Phase sanitisation | **Done** | See iteration 5 below |
| Baseline system | **Done** | EMA with motion-gated updates |
| Motion detection | **Done** | deltaRMS, NBVI selection |
| Dashboard presence indicator | **Done** | Implemented in dashboard/live.html and dashboard/js/app.js |
| CSI recording buffer | **Done** | Disk-backed circular buffer — mothership/internal/recording/buffer.go |
| Adaptive sensing rate | **Done** | RateController (2 Hz idle / 50 Hz active) — mothership/internal/ingestion/ratecontrol.go |

### Iteration Log

#### Iteration 5 — 2026-03-26

**Completed:** Signal processing package (phase sanitisation, baseline, motion detection)

Implemented the core signal processing pipeline in Go with:

- **Phase sanitization (`signal/phase.go`):**
  - Complex CSI computation: int8 I/Q → float64 complex → amplitude/phase
  - RSSI normalization (AGC compensation): `norm = 10^((rssi_ref - rssi)/20)`
  - Spatial phase unwrapping: correct 2π jumps between adjacent subcarriers
  - Linear regression (OLS): fit `phase = a*k + b` over data subcarriers
  - STO/CFO removal: residual phase = unwrapped - (slope × k + intercept)
  - HT20 subcarrier map: 64 total, 46 data (excluding null, guard, pilot)

- **Baseline system (`signal/baseline.go`):**
  - EMA baseline per link per subcarrier: `baseline = α×amplitude + (1-α)×baseline`
  - α = dt / (τ + dt) ≈ 0.0033 for dt=0.1s, τ=30s
  - Motion-gated updates: only update when smoothDeltaRMS < 0.05
  - Confidence scoring: 0.3 for stale baselines (>7 days), asymptotically → 1.0
  - Snapshot/restore for SQLite persistence
  - BaselineManager for multi-link coordination

- **Motion detection (`signal/features.go`):**
  - NBVI (Normalized Bandwidth Variance Index): `Var(amp) / Mean(amp)²`
  - Welford's online algorithm for numerically stable variance
  - Top-16 subcarrier selection by NBVI score
  - deltaRMS: `sqrt(mean((amp - baseline)²))` over selected subcarriers
  - Exponential smoothing: `smooth = 0.3×raw + 0.7×prev`
  - Motion threshold: smoothDeltaRMS > 0.02

- **Link processor (`signal/processor.go`):**
  - LinkProcessor: ties together phase sanitization, baseline, motion detection
  - ProcessorManager: manages per-link processors with thread-safe access
  - GetAllMotionStates(): returns motion state for all links
  - GetAllBaselines()/RestoreBaseline(): for SQLite persistence

**Constants used (from plan):**
- RSSIRefdBm = -30.0
- DefaultMotionThreshold = 0.05
- DefaultDeltaRMSThreshold = 0.02
- NBVITopCount = 16
- NBVIMinThreshold = 0.001
- DeltaRMSSmoothingAlpha = 0.3

**Tests:** 37 tests covering phase sanitization, baseline, NBVI, motion detection. All pass.

**Files created:**
```
mothership/internal/signal/
├── phase.go          — Phase sanitization algorithms
├── phase_test.go     — 15 tests for phase processing
├── baseline.go       — EMA baseline management
├── baseline_test.go  — 9 tests for baseline
├── features.go       — NBVI selection, deltaRMS, motion detection
├── features_test.go  — 13 tests for features
└── processor.go      — LinkProcessor, ProcessorManager
```

**Phase 2 Status:** COMPLETE

All Phase 2 items implemented — CSI recording buffer (mothership/internal/recording/buffer.go) and adaptive sensing rate (mothership/internal/ingestion/ratecontrol.go) landed after this iteration.

#### Iteration 4 — 2026-03-26

**Completed:** Docker packaging

Implemented container deployment with:

- **Dockerfile:** Multi-stage build for minimal production image
  - Stage 1: `golang:1.23-bookworm` builder with module caching
  - Stage 2: `distroless/static-debian12:nonroot` runtime (no shell, runs as UID 65532)
  - CGO_ENABLED=0 build (pure-Go SQLite compatible)
  - Dashboard static files copied to `/dashboard/`
  - Firmware directory as volume mount point (`/firmware/`)
  - Built-in healthcheck via wget

- **docker-compose.yml:** Production-ready orchestration
  - `network_mode: host` for mDNS multicast discovery (required for ESP32 nodes)
  - Volume mounts: `spaxel-data` for persistence, `./firmware` for OTA binaries
  - Environment variables: TZ, mDNS config, optional MQTT
  - Resource limits: 512m RAM, 2 CPUs (scalable to 1g for 16+ nodes)
  - 35s stop_grace_period for graceful shutdown
  - Ulimits: 4096/8192 file descriptors for node connections
  - Traefik labels (disabled by default, enable for TLS/proxy)

- **Supporting files:**
  - `VERSION` — 0.1.0 for image tagging
  - `.dockerignore` — Excludes docs, build artifacts, IDE files

**Key design decisions:**
- Host networking is REQUIRED for mDNS — Docker bridge blocks multicast 224.0.0.251
- distroless image minimizes attack surface (no shell, no package manager)
- Firmware binaries not bundled — users mount their own `/firmware` volume
- Traefik labels included but disabled — enable for production TLS

**Files created:**
```
Dockerfile
docker-compose.yml
VERSION
.dockerignore
```

**Phase 1 Status:** COMPLETE

All Phase 1 items implemented:
- ✅ ESP32 firmware skeleton
- ✅ Passive radar support
- ✅ BLE scanning
- ✅ Mothership WebSocket ingestion
- ✅ Dashboard skeleton
- ✅ Docker packaging

**Next:** Phase 2 — Signal Processing (baseline, deltaRMS, Fresnel zones)

#### Iteration 3 — 2026-03-26

**Completed:** Dashboard skeleton with 3D visualization

Implemented the web dashboard in vanilla JS + Three.js with:

- **Static file structure:** `dashboard/` directory with HTML/JS
  - `index.html` — Dark theme UI with status bar, node/link panels, amplitude chart
  - `js/app.js` — Main application (~350 lines)

- **3D Scene (Three.js):**
  - Ground grid (10×10m, 20 divisions)
  - OrbitControls for pan/zoom/rotate with damping
  - Axes helper for orientation
  - Responsive canvas with devicePixelRatio support
  - FPS counter in status bar

- **WebSocket connection (`/ws/dashboard`):**
  - Auto-reconnect with 3s backoff
  - JSON message handling for state, node events, link events
  - Binary CSI frame parsing (24-byte header + I/Q payload)
  - Connection status indicator (green/red dot)

- **Node/Link panels:**
  - Live list of connected nodes with MAC, firmware, chip
  - Active links with node:peer MAC pairs
  - Click-to-select for amplitude chart display

- **Amplitude chart (Canvas 2D):**
  - 64-bar visualization of subcarrier amplitudes
  - Real-time update from selected link's CSI frames
  - Color gradient based on amplitude intensity
  - Channel and RSSI display overlay

- **Dashboard package (Go):**
  - `internal/dashboard/hub.go` — Hub for client management and broadcasting
  - `internal/dashboard/server.go` — WebSocket handler with ping/pong keepalive
  - `internal/dashboard/hub_test.go` — 5 tests for hub operations
  - IngestionState interface for querying node/link state
  - CSIBroadcaster interface implementation

- **Main.go updates:**
  - Static file serving with SPA fallback
  - Dashboard WebSocket endpoint at `/ws/dashboard`
  - Auto-discovery of dashboard directory
  - Wiring between ingestion and dashboard packages

**Dependencies used (frontend):**
- Three.js r128 (CDN)
- OrbitControls (CDN)

**Tests:** 5 new tests for dashboard hub. All 27 tests pass.

**Files created:**
```
dashboard/
├── index.html
└── js/
    └── app.js

mothership/internal/dashboard/
├── hub.go
├── hub_test.go
└── server.go
```

**Remaining for Phase 1:**
- Docker packaging

#### Iteration 2 — 2026-03-26

**Completed:** ESP32-S3 firmware skeleton

Implemented the full ESP32 firmware in ESP-IDF C with:

- **Project structure:** Standard ESP-IDF layout with `firmware/` root
  - Top-level `CMakeLists.txt`, `sdkconfig.defaults`, `partitions.csv` (factory + OTA slots)
  - `main/` component with 5 source modules

- **State machine (main.c):** 7-state node lifecycle
  - BOOT → WIFI_CONNECTING → MOTHERSHIP_DISCOVERY → CONNECTED
  - Degraded states: WIFI_LOST, MOTHERSHIP_UNAVAILABLE, CAPTIVE_PORTAL
  - Exponential backoff on WiFi failures (1s → 30s max)
  - 10-failure threshold before captive portal

- **WiFi module (wifi.c/h):**
  - STA connection with exponential backoff
  - mDNS discovery for `_spaxel._tcp.local` with fallback to cached IP
  - Captive portal AP mode (`spaxel-XXXX`) with HTTP config page
  - Event-driven connection state via FreeRTOS event group

- **WebSocket client (websocket.c/h):**
  - Bidirectional communication on single connection
  - Binary CSI frame transmission (24-byte header + I/Q payload)
  - JSON message handling: hello, health, ble, ota_status upstream
  - Downstream command parsing: role, config, ota, reboot, identify, reject
  - OTA download task with progress reporting and automatic reboot

- **CSI capture (csi.c/h):**
  - WiFi promiscuous mode with CSI callback
  - Queue-based processing (32-frame buffer)
  - Passive mode BSSID filtering for radar
  - On-device amplitude variance tracking for motion hints (Welford's algorithm)
  - TX task for active probing

- **BLE scanner (ble.c/h):**
  - Passive BLE scanning on Core 0 (concurrent with WiFi)
  - Device cache (60 entries) with name and manufacturer data parsing
  - 5-second reporting interval via WebSocket
  - GAP event handling for advertisement processing

- **NVS persistence:** Full schema with 15 keys
  - WiFi credentials, node ID/token, mothership config
  - Role/rate persistence for degraded mode operation
  - Schema versioning for migration support

**Files created:**
```
firmware/
├── CMakeLists.txt
├── sdkconfig.defaults
├── partitions.csv
└── main/
    ├── CMakeLists.txt
    ├── spaxel.h
    ├── main.c
    ├── wifi.h / wifi.c
    ├── websocket.h / websocket.c
    ├── csi.h / csi.c
    └── ble.h / ble.c
```

**Phase 1 Status:** COMPLETE

All Phase 1 items implemented:
- ✅ ESP32 firmware skeleton
- ✅ Passive radar support
- ✅ BLE scanning
- ✅ Mothership WebSocket ingestion
- ✅ Dashboard skeleton
- ✅ Docker packaging

**Next:** Phase 2 — Signal Processing (baseline, deltaRMS, Fresnel zones)

#### Iteration 1 — 2026-03-26

**Completed:** Mothership WebSocket ingestion server

Implemented the core ingestion server in Go with:

- **Module structure:** `mothership/` with `cmd/mothership/` entrypoint and `internal/ingestion/` package
- **WebSocket endpoint:** `/ws/node` accepts bidirectional connections from ESP32 nodes
- **Binary frame parsing:** 24-byte header + variable payload, per spec in plan.md
  - Validation: min/max length, payload size match, channel validity (1-14), subcarrier limit (128)
  - Malformed frame tracking with warn/close thresholds (100/1000 per minute)
- **JSON message handling:** Parses hello, health, ble, motion_hint, ota_status
- **Per-link ring buffers:** 256-sample circular buffers keyed by `nodeMAC:peerMAC`
- **Connection lifecycle:** Node registration via hello, ping/pong keepalive (30s/60s), graceful shutdown
- **mDNS advertisement:** `_spaxel._tcp.local` via github.com/hashicorp/mdns
- **Role/config push:** Sends initial `rx` role and 20 Hz config on connect
- **Health endpoint:** `GET /healthz` returns `{"status":"ok","version":"..."}`

**Dependencies used:**
- `github.com/go-chi/chi` — HTTP routing
- `github.com/gorilla/websocket` — WebSocket server
- `github.com/hashicorp/mdns` — mDNS advertisement

**Tests:** 22 tests covering frame parsing, JSON messages, and ring buffer operations. All pass.

**Files created:**
```
mothership/
├── cmd/mothership/main.go
├── go.mod
├── go.sum
└── internal/ingestion/
    ├── frame.go
    ├── frame_test.go
    ├── message.go
    ├── message_test.go
    ├── ring.go
    ├── ring_test.go
    └── server.go
```

## Phase 7 — Learning & Analytics

Goal: The system gets smarter over time. User feedback drives improvement.

### Status: COMPLETE

| Item | Status | Notes |
|------|--------|-------|
| Detection feedback loop | **Done** | Thumbs up/down on detections |
| Self-improving localization | **Done** | BLE ground truth drives weight refinement |
| Presence prediction | **Done** | See iteration 6 below |
| Sleep quality monitoring | **Done** | Breathing analysis + motion scoring |
| Crowd flow visualization | **Done** | Trajectory accumulation into directional flow map |
| Anomaly detection & security mode | **Done** | 7-day pattern learning |

### Iteration 6 — 2026-04-09

**Completed:** Presence prediction for Home Assistant integration

Implemented the full presence prediction system with:

- **Per-person transition probability tracking (`prediction/model.go`):**
  - `zone_transitions_history` table with all zone transitions (person_id, from_zone, to_zone, hour_of_week, dwell_duration)
  - `transition_probabilities` table with Laplace-smoothed probabilities
  - `dwell_times` table with mean/stddev dwell time per person/zone/hour
  - `person_zone_entry` table for tracking current person positions
  - Zone transition recording via `PersonZoneChange()` method
  - Automatic probability recomputation with Laplace smoothing
  - Dwell time statistics with mean/stddev computation

- **Per-zone occupancy patterns (`prediction/accuracy.go`):**
  - `zone_occupancy_patterns` table with occupancy_prob per zone/hour_of_week
  - `zone_occupancy_history` table tracking entries/exits with timestamps
  - `recorded_predictions` table for tracking prediction accuracy
  - `accuracy_stats` table with rolling 7-day accuracy metrics
  - Zone occupancy pattern computation from historical data
  - Pattern-based occupancy prediction at target time

- **Time-slot based predictions (`prediction/horizon.go`):**
  - Monte Carlo simulation with 1000 runs for probabilistic predictions
  - Multi-step path simulation accounting for dwell times
  - Normal distribution sampling for dwell time variability
  - Horizon predictions at 5, 15, and 30 minutes
  - Returns probability distribution over all zones
  - Confidence scoring based on simulation agreement

- **HA sensor exposure (`mqtt/client.go`):**
  - `PublishPredictionSensors()` creates HA auto-discovery configs
  - `UpdatePredictionState()` publishes current predictions to MQTT
  - Three sensors per person:
    - `sensor.spaxel_<person>_predicted_zone` - zone name
    - `sensor.spaxel_<person>_prediction_confidence` - percentage
    - `sensor.spaxel_<person>_transition_minutes` - estimated minutes
  - Topics follow HA discovery pattern: `homeassistant/sensor/.../config`

- **REST API endpoints (`api/prediction.go`):**
  - `GET /api/predictions` - Get current predictions for all people
  - `GET /api/predictions?person=<id>&horizon=<min>` - Filtered predictions
  - `GET /api/predictions/stats` - Transition count, data age, model readiness
  - `POST /api/predictions/recompute` - Force probability recomputation
  - `GET /api/predictions/accuracy` - Per-person accuracy stats
  - `GET /api/predictions/accuracy/overall` - Overall system accuracy
  - `GET /api/predictions/accuracy/{personID}` - Person-specific accuracy
  - `GET /api/predictions/horizon` - Monte Carlo horizon predictions
  - `GET /api/predictions/horizon/{personID}` - Person-specific horizon prediction
  - `GET /api/predictions/patterns/zones` - Zone occupancy patterns
  - `GET /api/predictions/probabilities/{personID}` - Transition probabilities
  - `GET /api/predictions/samples/{personID}/zone/{zoneID}` - Sample counts

- **Main application wiring (`cmd/mothership/main.go`):**
  - Prediction module initialization (lines 492-534)
  - Zone transition recording on portal crossings (lines 1579-1581)
  - Provider wiring for zones, people, positions (lines 1831-1863)
  - MQTT client integration for prediction publishing (lines 1846-1863)
  - Periodic prediction update loop every 60 seconds (lines 1866-1888)
  - Periodic prediction evaluation every 30 seconds (lines 1931-1983)
  - REST API endpoint registration (lines 2527-2888)

**Constants and thresholds:**
- MinimumDataAge = 7 days (168 hours) before predictions activate
- MinimumSamplesPerSlot = 3 observations per time slot
- PredictionHorizon = 15 minutes (default)
- MonteCarloRuns = 1000 simulations
- TargetAccuracy = 75% at 15-minute horizon

**Accuracy tracking:**
- Records predictions when made (personID, currentZone, predictedZone, confidence, horizon)
- Evaluates pending predictions when target time is reached
- Compares predicted zone vs actual zone
- Computes rolling 7-day accuracy percentage
- Reports "meets_target" when accuracy ≥ 75% and min predictions threshold met

**Model learning:**
- Observations recorded every 5 minutes per person/zone
- EMA update: `p_new = p_old + α × (obs - p_old)` where α = 0.03
- Cold start: 7 days of data required for model readiness
- Slot ready when sample_count ≥ 3
- Automatic recomputation triggered on zone transitions

**Files created/modified:**
```
mothership/internal/prediction/
├── model.go           — ModelStore, transition probabilities, dwell times
├── predictor.go       — Predictor for presence prediction
├── horizon.go         — HorizonPredictor with Monte Carlo simulation
├── accuracy.go        — AccuracyTracker for prediction evaluation
├── history.go         — HistoryUpdater for zone transition recording
├── adapter.go         — Provider adapters for zones, people, positions
├── model_test.go      — Tests for model store operations
├── predictor_test.go  — Tests for prediction logic
├── accuracy_test.go   — Tests for accuracy tracking
└── horizon_test.go    — Tests for horizon predictions

mothership/internal/api/
├── prediction.go      — REST API handlers for predictions
└── prediction_test.go — Tests for prediction API endpoints

mothership/internal/mqtt/
└── client.go          — MQTT client with prediction sensor publishing
```

**Acceptance criteria met:**
- ✅ Per-person transition probability tracking - Full implementation with Laplace smoothing
- ✅ Per-zone occupancy patterns - Historical patterns with probability computation
- ✅ Time-slot based predictions - Monte Carlo simulation at configurable horizons
- ✅ HA sensor exposure for predicted states - Full auto-discovery with 3 sensors per person
- ✅ >75% accuracy at 15-minute horizon - AccuracyTracker with rolling 7-day window

**Phase 7 Status:** COMPLETE

## Phase 8 — Analysis & Developer Tools

Goal: Deep debugging, system tuning, and detection explainability.

### Status: COMPLETE

| Item | Status | Notes |
|------|--------|-------|
| Activity timeline (Component 27) | **Done** | Universal event stream with tap-to-time-travel |
| Detection explainability (Component 28) | **Done** | X-ray overlay, per-link contributions, BLE match details |
| Time-travel debugging | **Done** | Pause live, scrub timeline, parameter tuning overlay |
| Pre-deployment simulator | **Done** | Virtual space + nodes + walkers, GDOP overlay |
| CSI simulator (`cmd/sim`) | **Done** | Go CLI for hardware-free testing |
| Fresnel zone debug overlay | **Done** | Toggle wireframe ellipsoids in 3D scene |

### Implementation Summary

**Activity timeline (`internal/timeline/`):**
- `Storage` subscribes to EventBus and writes events to SQLite asynchronously via buffered queue
- Handles all event types: detections, zone transitions, alerts, system events, learning milestones
- WebSocket push of new events to dashboard clients in real-time
- REST API: `GET /api/events` with cursor-based pagination, filters (type/zone/person/time), and FTS5 search
- Frontend: `sidebar-timeline.js` — virtualized list, tap-to-time-travel, inline feedback buttons

**Detection explainability (`internal/explainability/`):**
- `Handler` maintains per-blob explanation data updated by the fusion engine
- Per-link contribution table: deltaRMS, Fresnel zone number, learned weight, contribution amount
- BLE match details: per-node RSSI, triangulation position, confidence
- Fresnel zone ellipsoid geometry for 3D visualization
- Frontend: `explainability.js` — X-ray overlay dims non-contributors, glowing contributing links

**Time-travel debugging (`internal/replay/`):**
- Append-only `csi_replay.bin` with per-frame records (recv_time_ms + raw CSI binary)
- `Worker` manages replay sessions: play/pause/seek/speed control
- Parameter tuning: `PATCH /api/replay/params` re-runs pipeline on buffered data at max speed
- "Apply to Live" copies tuned params to the running pipeline
- Dashboard: timeline scrubber integrated with activity timeline events

**Pre-deployment simulator (`internal/simulator/`):**
- `Space` defines room geometry with wall segments and material properties
- `NodeSet` manages virtual TX/RX node placement
- `WalkerSet` simulates random-walk or path-following persons
- Two-ray propagation model with path loss + wall penetration + first-order reflections
- GDOP computation per cell using Fisher information matrix
- REST API at `/api/simulator/*` for space/node/walker management and GDOP computation

**CSI simulator CLI (`cmd/sim/`):**
- Connects to mothership as virtual nodes, sends synthetic CSI binary frames
- Walker random-walk model with Gaussian velocity updates, wall reflection
- Amplitude/phase generated from propagation model with Gaussian noise injection
- Optional BLE advertisement simulation (`--ble` flag)
- Verified authentication (exits non-zero on `{type:"reject"}`)
- Integration test support: polls `GET /api/blobs` for blob count assertions

**Fresnel zone debug overlay (`dashboard/js/fresnel.js`):**
- Toggle-able wireframe ellipsoids between active TX/RX link pairs
- Ellipsoid geometry computed from TX/RX positions and Fresnel zone number
- Color-coded by zone number (zone 1 = bright, outer zones = dimmer)
- Toolbar button integration with existing layers panel

**Files created/modified:**
```
mothership/internal/timeline/       — timeline.go, timeline_test.go, handler.go, buffer_adapter.go
mothership/internal/explainability/ — handler.go
mothership/internal/replay/        — engine.go, engine_test.go, pipeline.go, pipeline_test.go,
                                     session.go, store.go, store_test.go, types.go, worker.go
mothership/internal/simulator/     — accuracy.go, engine.go, gdop.go, handler.go, node.go,
                                     physics.go, propagation.go, registry_bridge.go, session.go,
                                     space.go, virtual_state.go, walker.go + test files
mothership/cmd/sim/                 — main.go, generator.go, verify.go, walker.go, main_test.go
mothership/internal/api/            — replay.go, replay_test.go, simulator.go
dashboard/js/                       — explainability.js, replay.js, fresnel.js, sidebar-timeline.js
```

**Phase 8 Status:** COMPLETE

## Phase 9 — UX Polish & Accessibility

Goal: Accessible to every household member. Power user efficiency. Always-on ambient display.

### Status: COMPLETE

| Item | Status | Notes |
|------|--------|-------|
| Simple mode (progressive disclosure) | **Done** | Card-based mobile-first UI, room cards, activity feed |
| Ambient dashboard mode (Component 31) | **Done** | `/ambient` route, Canvas 2D, time-of-day palette, auto-dim |
| Spatial quick actions (Component 32) | **Done** | Right-click / long-press context menus on 3D elements |
| Command palette (Component 34) | **Done** | Ctrl+K / Cmd+K, fuzzy search, navigate time, execute commands |
| Morning briefing (Component 35) | **Done** | Daily summary card on first open, push notification support |
| Guided troubleshooting (Component 36) | **Done** | Proactive contextual help, dismissible, never repeats |
| Mobile-responsive expert mode | **Done** | Touch orbit/pan/zoom, hamburger menu, FXAA on mobile |
| Fleet status page | **Done** | Full table with OTA progress, bulk actions, camera fly-to |

### Implementation Summary

**Simple mode (`dashboard/simple.html`, `dashboard/js/simple.js`):**
- Card-based layout: one room card per zone with occupancy count and person names
- Activity feed as chronological event list from timeline REST API
- Alert banner for fall detection, anomaly alerts, system warnings
- Sleep summary card for morning view
- No 3D scene — designed for non-technical household members
- Toggle button in toolbar; per-user default stored in `localStorage`

**Ambient dashboard (`dashboard/ambient.html`, `dashboard/js/ambient.js`):**
- Served at `/ambient` — separate lightweight route for wall-mounted tablets
- Canvas 2D renderer: colored circles for people, zone labels, soft glow effects
- Time-of-day palette: morning (cool), day (neutral), evening (amber), night (very dim)
- Auto-dim when house empty for 30+ min; gentle fade-in on person detection
- Alert mode: pulsing red border + large text + action buttons on fall/security events
- Morning briefing integration: briefing text shown on first detection, fades after 30 s
- Reconnect backoff handles WebSocket drops gracefully

**Spatial quick actions (`dashboard/js/quick-actions.js`):**
- Three.js Raycaster determines target under cursor (person/node/zone/portal/trigger/empty)
- Context menu component renders appropriate options per target type
- Per-blob: "Who is this?", "Why?", "Follow" camera, "Create automation", "Mark incorrect"
- Per-node: "Diagnostics", "Blink LED", "Reposition", "Update firmware", "Show links"
- Per-empty: "What happened here?", "Add trigger zone", "Add virtual node", "Coverage quality"
- Per-zone: "Zone history", "Edit zone", "Create automation", "Crowd flow"
- "Follow" mode: camera smoothly tracks person, auto-orbiting to keep them centered

**Command palette (`dashboard/js/command-palette.js`):**
- Ctrl+K (Cmd+K on Mac) opens universal search and command interface
- Fuzzy matching across zones, persons, nodes, settings, help topics, events
- Navigate time: "last night 2am", "yesterday kitchen", "this morning"
- Execute commands: "update all nodes", "re-baseline kitchen", "arm security"
- Help: "help fall detection", "why false positive", "troubleshoot kitchen"
- Recently used commands surface first; expert mode only

**Morning briefing (`internal/briefing/`, `dashboard/js/briefing.js`):**
- `Generator` assembles briefing in priority order from sleep, events, anomalies, system health
- Priority blocks: critical alerts → sleep summary → who is home → overnight anomalies → system health → predictions → learning progress
- Degenerate case: "All quiet last night. All systems healthy."
- Stored in `briefings` SQLite table (one per day per person)
- REST API: `GET /api/briefing`, `GET /api/briefing/current`
- Dashboard: card overlay on first open, dismissible, slides away after 10 s

**Guided troubleshooting (`internal/guidedtroubleshoot/`, `dashboard/js/troubleshoot.js`):**
- Trigger conditions: detection quality drops, repeated setting changes, node offline >2 h
- Repeated-edit detection: per-key counter in memory; hint delivered in `GET /api/settings` response as `"repeated_edit_hint": true`
- Proactive diagnostic flow: check connectivity → show link health → suggest repositioning → offer re-baseline
- First-time feature tooltips shown once, dismissed on click, stored in `localStorage`
- Post-feedback explanations in timeline after incorrect/missed detection feedback

**Mobile-responsive expert mode (`dashboard/js/mobile.js` + CSS):**
- Touch orbit (one finger), pan (two fingers), pinch-zoom (two fingers)
- FXAA anti-aliasing replaces MSAA on mobile (better performance on low-power GPUs)
- Hamburger menu for toolbar panels on small screens
- Viewport meta tag with `viewport-fit=cover` for notch-aware layout
- CSS Grid layout collapses gracefully on narrow viewports

**Fleet status page (`dashboard/fleet.html`, `dashboard/js/fleet-page.js`):**
- Full table with sortable columns: Name, MAC, Role, Position (fly-to), Firmware, RSSI, Status, Uptime, Actions
- OTA progress bar per node during updates (PENDING → DOWNLOADING → REBOOTING → VERIFIED / FAILED)
- Bulk actions: Update All (rolling OTA), Re-baseline All, Export Config, Import Config
- Camera fly-to: clicking position coordinates in table jumps 3D camera to that node
- Responsive layout works on both desktop and tablet

**Files created/modified:**
```
dashboard/simple.html               — Simple mode page
dashboard/ambient.html              — Ambient dashboard page
dashboard/fleet.html                — Fleet status page
dashboard/js/simple.js              — Simple mode WebSocket + card rendering
dashboard/js/simplemode.js          — Simple mode toggle and state management
dashboard/js/ambient.js             — Ambient dashboard controller
dashboard/js/ambient_renderer.js    — Canvas 2D renderer with time-of-day palette
dashboard/js/ambient_briefing.js    — Morning briefing integration for ambient
dashboard/js/quick-actions.js       — Context menu for 3D scene elements
dashboard/js/command-palette.js     — Universal search and command interface
dashboard/js/briefing.js            — Morning briefing card overlay
dashboard/js/troubleshoot.js        — Guided troubleshooting UI
dashboard/js/guided-help.js         — First-time feature discovery tooltips
dashboard/js/mobile.js              — Mobile touch controls and FXAA
dashboard/js/fleet-page.js          — Fleet status table and bulk actions
mothership/internal/briefing/       — briefing.go, briefing_test.go, scheduler.go,
                                     dashboard_adapter.go, notify_adapter.go
mothership/internal/guidedtroubleshoot/ — discovery.go, notifier.go, quality.go,
                                          linkweather.go, reposition.go, alert_handler.go
mothership/internal/api/            — briefing.go, briefing_test.go, guided.go
```

**Phase 9 Status:** COMPLETE

## Hardware-Free Runtime Boot

Established 2026-07-07 (bead bf-3zll, decomposed from bf-40hc "hardware-free
runtime blob path"). The mothership builds and boots to a healthy state with
**no ESP32 hardware** and no Docker — the first link in the hardware-free
runtime chain. The canonical path is exercised by the e2e harness
(`mothership/tests/e2e/e2e_test.go`, `TestHarness.Start`); the commands below
are the human-runnable equivalent.

### Build (from a clean checkout, no build tags)

```bash
cd mothership                          # module github.com/spaxel/mothership
go build -o /tmp/spaxel-mothership ./cmd/mothership
```

No `-tags=embed` is needed locally: that tag embeds `cmd/mothership/dashboard/`,
which is only populated during the Docker build (`COPY dashboard/ ...`). A plain
build omits the bundled dashboard (`[WARN] Dashboard directory not found: /dashboard`,
harmless) — `/healthz` and all `/api/*` endpoints work regardless.

### Boot to /healthz

```bash
DATADIR=$(mktemp -d /tmp/spaxel-data-XXXX)
SPAXEL_BIND_ADDR=127.0.0.1:8080 \
SPAXEL_DATA_DIR="$DATADIR" \
SPAXEL_LOG_LEVEL=info \
TZ=UTC \
  /tmp/spaxel-mothership &

# Verify (reaches ok in <1s; budget is ~15s per HealthTimeout):
curl -fsS http://localhost:8080/healthz
# -> {"status":"ok","uptime_s":0,"version":"dev","nodes_online":0,"db":"ok","shedding_level":0}

# Stop:
kill %1   # SIGINT triggers the ordered 30s graceful shutdown
```

### Verified behavior

- `/healthz` returns `{"status":"ok",...}` in ~230 ms with the process alive.
- All 7 startup phases complete (`[READY] All 7 phases completed in 231ms`).
- No panic/crash in stderr. Two benign WARN lines are expected on first boot:
  the process's own startup self-probe can race the listener bind
  (`Health check failed: ... connection refused (continuing anyway)`) and the
  dashboard is unbundled without the `embed` tag.

## Hardware-Free Simulator → CSI Streaming (no reject)

Established 2026-07-07 (bead bf-3hji, decomposed from bf-40hc; second link —
builds on the bf-3zll healthy-mothership boot above). With the mothership
healthy, `spaxel-sim` connects a 4-node fleet and streams synthetic CSI without
being rejected. Canonical path: `TestHarness.RunSimulator` in
`mothership/tests/e2e/e2e_test.go`; the commands below are the human-runnable
equivalent.

### Build the simulator (from the mothership module root)

```bash
cd mothership                          # module github.com/spaxel/mothership
go build -o /tmp/spaxel-sim ./cmd/sim
```

`./cmd/sim` from the module root resolves to `mothership/cmd/sim` (the in-module
simulator that supports `--ble`/`--seed`/`--scenario` and the WebSocket hello +
CSI streaming path). It is the same binary the e2e harness builds
(`exec.Dir = mothership/`, `./cmd/sim`).

### Run against the healthy mothership (exact command)

`--provision` (default) is shown explicitly below: it mints a REAL per-node HMAC
token via `POST /api/provision`, so the fleet connects under
`SPAXEL_MIGRATION_WINDOW_HOURS=0` (strict) exactly as well as the open default.
Drop it for `--provision=false` (legacy dummy token, window-dependent) or pin a
token with `--token <t>` (see "Auth policy" below).

```bash
SPAXEL_MIGRATION_WINDOW_HOURS=0 ./mothership   # then, separately:
/tmp/spaxel-sim \
  --mothership ws://localhost:8080/ws/node \
  --provision \
  --nodes 4 --walkers 1 --rate 20 --duration 30 --ble --seed 42
```

### Auth policy — window-independent (bf-4mle6)

The hardware-free build authenticates on **real per-node HMAC tokens**, not the
migration window. By default `spaxel-sim` is `--provision` (true):

- `resolveTokens()` mints a REAL per-node token for every virtual node:
  `provisionNodeToken()` POSTs `{"mac":<mac>}` to the mothership
  `/api/provision`, which returns `node_token = HMAC-SHA256(installSecret, mac)`
  (`cmd/sim/main.go`). The token is presented as `X-Spaxel-Token` on each node's
  WS dial.
- The ingestion server (`bf-1o7qi`) bridges that header into `hello.Token`
  (`server.go:524-527`) — the validator reads only `hello.Token` — and the
  always-wired validator (`main.go:4494` `SetTokenValidator(provSrv.ValidateToken)`)
  accepts a valid token **regardless of the migration window**
  (`server.go:537` `tokenOK := hello.Token != "" && validator(hello.MAC, hello.Token)`).
  Only an empty/invalid token falls through to the window branch
  (`server.go:538-549`), so a provisioned node is accepted and paired under
  `SPAXEL_MIGRATION_WINDOW_HOURS=0` (strict) exactly as well as 24 or larger.

The e2e harness proves this by running under `SPAXEL_MIGRATION_WINDOW_HOURS=0`
in `TestHarness.Start` (`bf-qzrmq`, commit 250056c): the no-reject path is
exercised against the STRICT window, not the open 24h default, so it is not a
side effect of the implicit window.

`--token <t>` overrides everything and applies `<t>` verbatim to every node
(not window-independent unless `<t>` is itself a valid HMAC token).
`--provision=false` is the legacy dummy-token path: nodes are admitted only
while the migration window is open and REJECTED once it closes — explicitly NOT
window-independent, kept only to reproduce the pre-fix behaviour documented in
the bf-2hdbg write-up below.

### Verified behavior (4 nodes, 1 walker, 20 Hz, 30 s, --ble, --seed 42)

- All 4 nodes connect and receive roles (`tx`/`tx`/`rx`/`rx`); sim exits 0.
- **No REJECT, no HTTP 401/403** anywhere — 0 `reject` lines in the sim log and
  0 auth/policy-rejection lines in the mothership log. The mothership logs
  `[INFO] Node connected: MAC=AA:BB:CC:00:00:0[0-3] firmware=sim-1.0.0` for all
  four and the fleet optimises roles (coverage 0.0% -> 23.1%).
- **frames/s > 0**: `[SIM] Stats` reports `fps=238.8` (10s) -> `239.6` (30s);
  final `Frames sent: 7194, Average FPS: 239.5` (= 12 node-pairs × 20 Hz).
- **/api/nodes lists the sim nodes online mid-run**: `/healthz` reports
  `nodes_online: 4`; `/api/nodes` returns 4 rows with fresh `last_seen_at` and a
  zero `went_offline_at` (i.e. online), at real perimeter positions
  `(0,0,2)`/`(5,0,2)`/`(5,5,2)`/`(0,5,2)`.

### Independently re-verified (bf-4ads8, third chain link)

Re-ran the exact documented command against a fresh `bf-3zll` healthy mothership
(clean `mktemp` data dir; `--provision` left at default — the real-token path is
window-independent per "Auth policy" above) on 2026-07-07:

```bash
/tmp/spaxel-sim --mothership ws://localhost:8080/ws/node \
  --nodes 4 --walkers 1 --rate 20 --duration 30 --ble --seed 42
```

- All 4 nodes connect (`[SIM] Node 0..3: connected to mothership`; mothership
  `[INFO] Node connected` for all four MACs `02:53:AC:00:00:0[0-3]`; sim exits 0).
- **No REJECT** — 0 `reject` lines in the sim log and 0 auth/policy-rejection
  lines in mothership stderr.
- **frames/s > 0** — `[SIM] Stats` per-second `frames/s≈232..246`; final
  `Frames sent: 7200, Duration: 30.0 s, Average FPS: 240.0` (12 ordered
  node-pairs × 20 Hz).
- Independent mothership-side check: `/healthz` mid-run reports
  `nodes_online: 4`; `/api/nodes` lists the four sim MACs with live
  `last_seen_at` and a zero `went_offline_at`.

Reproduces bf-3hji's connect + stream + no-reject result. `blobs: 0` remains out
of scope here (next chain link, bf-4q5w / IO-6 hard-gate).

### bf-vuzie — /api/nodes lists the 4 sim nodes online (final chain gate)

Verified the last acceptance gate for the chain and parent bf-3hji: during a sim run
the mothership REST API lists the 4 sim nodes and each is online per the e2e-harness
definition of "online" — `GetNodes` computes `now - last_seen_at < 30s`
(`mothership/tests/e2e/e2e_test.go`). Run on 2026-07-07 against a fresh strict-window
mothership (`SPAXEL_MIGRATION_WINDOW_HOURS=0`, clean `mktemp` data dir):

```bash
go build -o /tmp/spaxel-sim ./cmd/sim        # repo-root go.work module — see trap below
go build -o /tmp/mothership ./mothership/cmd/mothership
SPAXEL_MIGRATION_WINDOW_HOURS=0 SPAXEL_BIND_ADDR=127.0.0.1:8080 \
  SPAXEL_DATA_DIR=$(mktemp -d) TZ=UTC /tmp/mothership &
/tmp/spaxel-sim --mothership ws://localhost:8080/ws/node \
  --nodes 4 --walkers 1 --rate 20 --duration 30 --ble --seed 42
# ~9s into the run:
curl -s http://localhost:8080/api/nodes
```

`GET /api/nodes` mid-run returns **4 rows**, all online (`went_offline_at` = zero
value on every row):

`now` at probe time was `2026-07-07T19:41:12.042Z` (`date -u`, captured in the same
shell as the `/api/nodes` curl).

| mac | role | pos (x,y,z) | last_seen_at | Δ to now | online (<30s) |
|-----|------|-------------|--------------|----------|---------------|
| 02:53:AC:00:00:00 | tx | (0,0,2.5) | 2026-07-07T19:41:03.071Z | 8.97s | ✅ |
| 02:53:AC:00:00:01 | tx | (6,0,2.5) | 2026-07-07T19:41:03.064Z | 8.98s | ✅ |
| 02:53:AC:00:00:02 | rx | (0,5,2.5) | 2026-07-07T19:41:03.077Z | 8.97s | ✅ |
| 02:53:AC:00:00:03 | rx | (6,5,2.5) | 2026-07-07T19:41:03.078Z | 8.96s | ✅ |

- `/healthz` mid-run: `{"status":"ok","uptime_s":19,"nodes_online":4,"db":"ok",...}`.
- Sample row: `{"mac":"02:53:AC:00:00:01","role":"tx","pos_x":6,"pos_y":0,"pos_z":2.5,
  "virtual":false,"first_seen_at":"2026-07-07T19:41:03.040Z",
  "last_seen_at":"2026-07-07T19:41:03.064Z","went_offline_at":"0001-01-01T00:00:00Z",
  "firmware_version":"sim-1.0.0","chip_model":"ESP32-S3","health_score":0}`.
- Provisioning path proven live: 4× `POST /api/provision → 200`, real per-node HMAC
  tokens minted, 4× `[SIM] Node N: connected to mothership` and 4× mothership
  `[INFO] Node connected` (one per MAC), **0** `reject`/`invalid_token`/`401`/`403`
  in either log, and `Frames sent: 7200` (= 12 ordered node-pairs × 20 Hz × 30 s) —
  the fleet is admitted on real tokens under the STRICT window, not the open 24h default.

All three bf-vuzie acceptance criteria hold: `/api/nodes` returns the 4 sim nodes;
each `last_seen_at` is within 30s of now (online per harness logic); sample JSON +
node count recorded above.

#### Sim-binary trap — why a naive reproduction gets 0 nodes

The repo has **two** `cmd/sim` source trees; only the repo-root one connects under
the strict window:

- **`cmd/sim/` (repo root — go.work module `github.com/spaxel/sim`)** — the canonical,
  current sim. `apiBaseFromMothership` drops the WS path, so provisioning POSTs to
  `http://localhost:8080/api/provision` (200) and nodes connect on real tokens.
  **This is the one to build** (`go build -o /tmp/spaxel-sim ./cmd/sim` from the repo
  root).
- **`mothership/cmd/sim/` (in-module — part of the mothership module, a `main.go.bak`
  sits next to it)** — a stale copy. Its `apiBaseFromMothership` does NOT drop the
  path, so it POSTs to `http://localhost:8080/ws/node/api/provision` → **404**;
  provisioning fails, it falls back to a token the strict-window validator rejects
  (`[WARN] Node ... rejected: invalid_token`), and `/api/nodes` returns `[]`.

So the older instruction `cd mothership && go build -o /tmp/spaxel-sim ./cmd/sim`
(quoted in the "Run against the healthy mothership" block above and inherited from
bf-3hji/bf-4ads8) now builds the STALE in-module sim and reproduces the 0-node
failure. Build the repo-root `cmd/sim` instead. The `tests/e2e` harness has the same
mismatch: `TestHarness.RunSimulator`/`moduleRoot()` resolve `./cmd/sim` to
`mothership/cmd/sim`, so the e2e suite is RED at HEAD under the strict window
(`TestSimulatorConnection`, `TestConcurrentNodes`, `TestFullE2EIntegration`,
`TestIO6HardGate_WalkerProducesTrackedBlob` all observe 0 online nodes; `go vet ./...`
is clean and all non-e2e unit tests pass). `TestConcurrentNodes` additionally uses
`SimulateNode`, which dials with no token at all and is rejected. These are
pre-existing (introduced by the in-flight sim migration, not by this bead) and belong
to a dedicated harness/sim-consolidation fix; they do not affect the direct
`/api/nodes` verification above, which is green.

### Out of scope (tracked separately)

- The sim reports `blobs detected: 0`. Blob production is the next link in the
  chain — the fusion `SetNodePosition` wiring gap (bf-4q5w / IO-6 hard-gate).
  This bead proves connect + stream + no-reject, which is green; the blob gate
  is deliberately RED until bf-4q5w and is not weakened here.

### bf-4iewr — REJECT/token umbrella verified (strict window, negative control)

The umbrella goal of bf-4iewr — *the hardware-free mothership does NOT reject
spaxel-sim nodes* — is verified end-to-end against the STRICT window
(`SPAXEL_MIGRATION_WINDOW_HOURS=0`, so `SetMigrationDeadline` is never called
and the deadline stays zero-value). The fixes live at HEAD: the `bf-1o7qi`
header bridge (`server.go:524-527`) + `bf-4mle6` real-token provisioning.

Verified directly (mothership `mothership/mothership`, sim `cmd/sim/sim`,
both built from source; 2026-07-07):

- **Positive (default `--provision`):** `--nodes 2 --walkers 0 --duration 12`.
  Both nodes provision a real HMAC token, connect, receive `role`+`config`, and
  stream CSI at ~40 frames/s. Mothership logs `[INFO] Node connected` for both.
  **Zero** `reject`/`invalid_token`/`401`/`403` in mothership stderr. Pass.
- **Negative control (`--token bogus...`):** the same strict-window mothership
  **rejects** the bogus-token node — `[WARN] Node 02:53:AC:00:00:00 rejected:
  invalid token` — and the sim exits on reject. This proves the validator is
  **live and enforcing** under the strict window: legitimate sim nodes connect
  on the strength of their real tokens, not because auth was disabled. Pass.

Both acceptance criteria of bf-4iewr hold: root cause confirmed (validator wired
unconditionally at `main.go:4494`; the pre-`bf-1o7qi` dead header path is fixed),
and zero REJECT in mothership stderr when the sim connects.

## bf-2hdbg — The migration window (not tokens) is what accepts sim nodes

> **RESOLVED 2026-07-07 (bf-1o7qi + bf-4mle6).** This finding was accurate
> *before the token path was fixed*: at the time the sim was genuinely tokenless
> to the validator and admission was purely the 24h window. The fix landed
> exactly where this bead predicted — on the token/supply side: `bf-1o7qi`
> bridges the `X-Spaxel-Token` header into `hello.Token`, and `bf-4mle6` makes
> `spaxel-sim` provision a REAL per-node HMAC token (`--provision`, default) — so
> a sim node is now accepted on a valid token **regardless of the window** (see
> "Auth policy" above). The detailed analysis below is preserved verbatim as the
> historical root-cause record; its conclusion is superseded by the resolution at
> the end of this section.

Established 2026-07-07 (bead bf-2hdbg, third link of the bf-34lwt split; builds
on the bf-3hji "no reject" finding above). The bf-3hji "no REJECT, sim nodes
accepted" result is **not** a real fix — it is a side effect of the 24h
migration window that a fresh boot opens. Tokens have nothing to do with it:
sim nodes are genuinely tokenless from the validator's perspective (the `token`
they set is the *header*, which is the dead path per bf-29wyl; the validator
checks only `hello.Token` in the message body per bf-5ig3e, and the sim's hello
body carries no `token` field). The only thing admitting them is the migration
deadline. This pins down exactly when REJECT fires.

### Sim nodes are tokenless to the validator

- The sim's hello body (`mothership/cmd/sim/main.go:652-664`) carries
  `type/mac/firmware_version/capabilities/chip/flash_mb/uptime_ms/wifi_rssi/ip/pos_*
  ` and **no `token` field**. The token is set only as the `X-Spaxel-Token`
  HTTP header (`mothership/cmd/sim/main.go:633`) — the unread header path.
- The validator checks the body field only —
  `mothership/internal/ingestion/server.go:513`:
  `tokenOK := hello.Token != "" && validator(hello.MAC, hello.Token)`, where
  `hello.Token` maps `json:"token,omitempty"` (`message.go:22`). With no body
  token, `tokenOK` is `false` for every sim node, unconditionally.

### Acceptance is the migration window, nothing else

- `mothership/internal/ingestion/server.go:510-518` is the acceptance branch:

  ```
  510:	deadline := s.migrationDeadline
  ...
  513:		tokenOK := hello.Token != "" && validator(hello.MAC, hello.Token)
  514:		if !tokenOK {
  515:			if !deadline.IsZero() && time.Now().Before(deadline) {
  516:				log.Printf("[WARN] Node %s connected without valid token (migration window open until %s)",
  517:					hello.MAC, deadline.Format(time.RFC3339))
  518:				nc.Unpaired = true
  ```

  The `tokenOK` value is irrelevant to admission here — a tokenless node is
  admitted as long as line 515 holds (`deadline` set AND now-before-deadline).
  That is the bf-3hji "no reject" path: the node is accepted and flagged
  `Unpaired`.

- The window defaults to 24h. `mothership/internal/config/config.go:139`
  `cfg.MigrationWindowHours = 24` (default before env override), field comment
  at `config.go:43` "default 24, 0 = disabled". On a fresh boot the deadline is
  computed and installed at `mothership/cmd/mothership/main.go:4495-4497`:

  ```
  4495:	if cfg.MigrationWindowHours > 0 {
  4496:		deadline := time.Now().Add(time.Duration(cfg.MigrationWindowHours) * time.Hour)
  4497:		ingestSrv.SetMigrationDeadline(deadline)
  ```

  Fresh boot + default 24 → deadline = now+24h → line 515 true for 24h → sim
  nodes accepted as `Unpaired`, no reject (matches bf-3hji). Tokens never
  entered the picture.

### REJECT fires only when the window is closed

- The reject branch is the `else` of line 515 —
  `mothership/internal/ingestion/server.go:519-526`:

  ```
  519:			} else {
  520:				if hello.Token == "" {
  521:					log.Printf("[WARN] Node %s rejected: missing token", hello.MAC)
  522:				} else {
  523:					log.Printf("[WARN] Node %s rejected: invalid token", hello.MAC)
  524:				}
  525:				s.sendReject(conn, "invalid_token")
  526:				conn.Close()
  ```

  `sendReject` writes `{"type":"reject","reason":"invalid_token"}`
  (`mothership/internal/ingestion/server.go:841-845`) and the caller closes the
  connection.

- This `else` is reached when line 515 is false, i.e. either:
  - `SPAXEL_MIGRATION_WINDOW_HOURS=0` → `main.go:4495` guard is false →
    `SetMigrationDeadline` is never called → `migrationDeadline` stays at its
    zero value → `deadline.IsZero()` is true → line 515 false → reject branch
    (strict mode from startup, per the `config.go:137` comment "0 = disabled");
    OR
  - uptime > 24h → `time.Now().Before(deadline)` is false → line 515 false →
    reject branch.

  In both cases every tokenless sim node logs "rejected: missing token" and is
  sent `invalid_token` then disconnected.

### Conclusion — RESOLVED by the provisioning fix (was: "a 24h-window mask")

**Status: RESOLVED (bf-1o7qi + bf-4mle6).** The original conclusion below was
correct as of the bead's investigation and correctly identified the required
fix direction ("the real fix is on the token/supply side"). That fix has now
landed:

- `bf-1o7qi` (`server.go:524-527`) bridges the `X-Spaxel-Token` header into
  `hello.Token`, so a header-only client (the sim, and firmware following the
  plan.md auth contract) is no longer tokenless to the validator.
- `bf-4mle6` makes `spaxel-sim` provision a REAL per-node HMAC token via
  `--provision` (default), presented as `X-Spaxel-Token` and validated
  server-side by the always-wired validator (`server.go:537`) **regardless of
  the migration window**.

A provisioned sim node is therefore accepted and paired under
`SPAXEL_MIGRATION_WINDOW_HOURS=0` (strict) exactly as well as 24h or larger —
the e2e harness runs under the strict window (`bf-qzrmq`) to prove it. The
window branch (`server.go:538-549`) now admits only the explicit legacy
dummy-token path (`--provision=false`), which remains deliberately NOT
window-independent so the pre-fix behaviour can still be reproduced.

---

*Historical conclusion (pre-fix, preserved verbatim):*

The then-green "sim nodes connect without REJECT" state was entirely a property
of the default 24h migration window that a fresh boot opens. It was not a fix
to the token/auth path: sim nodes remained tokenless in the only place checked
(`hello.Token`), and the acceptance decision was gated purely on the deadline,
not on token validity. REJECT returned the moment the window was closed — set
`SPAXEL_MIGRATION_WINDOW_HOURS=0`, or let the mothership run past 24h of
uptime, and every tokenless sim node was sent `invalid_token` and disconnected.
The real fix — identified here and since implemented in bf-1o7qi + bf-4mle6 —
is on the token/supply side (provision a real token into the hello body, or fix
the dead header-read path), not on the window.

