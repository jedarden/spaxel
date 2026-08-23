# Spaxel Repository Structure Catalog

**Generated:** 2026-08-23  
**Purpose:** Complete directory structure documentation for build trigger classification

## Repository Overview

Spaxel is a WiFi CSI-based indoor positioning system comprising:
- **Firmware:** ESP32-S3 C code (ESP-IDF)
- **Mothership:** Go backend with SQLite persistence
- **Dashboard:** Vanilla JS + Three.js web UI
- **Simulator:** Go CLI for testing without hardware

---

## Root Level Files

### Configuration & Build
- `Dockerfile` — Multi-stage build (ESP-IDF firmware → Go binary → distroless runtime)
- `docker-compose.yml` — Single-service deployment manifest
- `go.work` — Go workspace stitching 3 modules
- `go.work.sum` — Go workspace dependencies
- `VERSION` — Single source of truth for release version
- `.golangci.yml` — Lint configuration

### NEEDLE Fleet Management
- `.needle.yaml` — NEEDLE workspace configuration
- `.needle-predispatch-sha` — Pre-dispatch SHA tracking

### Project Documentation
- `README.md` — Project overview and quickstart
- `PROGRESS.md` — Implementation progress tracking (54KB)
- `LICENSE` — MIT License
- `API_IMPLEMENTATION_STATUS.md` — REST API completion status
- `BLE_PERSONID_INVESTIGATION.md` — BLE identity investigation notes
- `GDOP_COMPUTATION_GUIDE.md` — GDOP algorithm documentation
- `gdop_search_results.md` — GDOP code search results
- `console-implementation-status.md` — Console routing implementation status

### Scripts & Utilities
- `blob_observation.sh` — Blob observation helper
- `window_test.sh` — Window testing script
- `fix_ble_handlers.py` — BLE handler fix script

### Git & Build Artifacts
- `.gitignore` — Git exclusions
- `.gitattributes` — Git attributes
- `.dockerignore` — Docker exclusions

---

## `/firmware` — ESP32-S3 Firmware (ESP-IDF C)

```
firmware/
├── main/                          # Primary application component
│   ├── main.c                    # App entry point, state machine
│   ├── spaxel.h                   # Main header
│   ├── wifi.c / wifi.h           # WiFi station, mDNS, captive portal
│   ├── csi.c / csi.h             # CSI capture and frame serialization
│   ├── websocket.c / websocket.h # WebSocket client (binary + JSON)
│   ├── ble.c / ble.h             # BLE passive scan (Core 0)
│   ├── led.c / led.h             # LED control (identify, OTA progress)
│   ├── provision.c / provision.h # Serial provisioning (10s window)
│   ├── nvs_migration.c / nvs_migration.h # NVS schema migration
│   ├── ntp.c / ntp.h             # NTP sync for TX stagger slots
│   ├── transport.c / transport.h # Transport layer abstraction
│   ├── version.h.in              # Version template (filled at build)
│   ├── CMakeLists.txt            # Component build config
│   └── idf_component.yml        # ESP-IDF component manifest
│
├── test/                          # Host-based tests (gcc harness, no ESP-IDF)
│   ├── test_runner.c / test_runner.h    # Test framework
│   ├── test_sanity.c                   # Sanity checks
│   ├── test_csi_frame.c               # Binary frame format tests
│   ├── test_serial_prov.c             # Provisioning JSON parser + fuzz
│   ├── test_nvs_migration.c           # NVS schema migration tests
│   ├── test_console_config.c          # Console routing config tests
│   ├── test_ota_during_wifi_reconnect.c  # OTA + WiFi race tests
│   ├── test_wifi_restart_race.c       # WiFi restart race tests
│   ├── test_all_restart_trigger_points.c  # Startup trigger point tests
│   ├── OTA_RECONNECT_TEST_RESULTS.md  # Test results docs
│   ├── OTA_DURING_RECONNECT_TEST_RESULTS.md
│   ├── Makefile                       # gcc harness runner
│   └── build/                         # Compiled test artifacts
│
├── scripts/                       # Firmware utilities
│   ├── generate-signing-key.sh   # OTA signing key generation
│   ├── sign-firmware.sh          # OTA binary signing
│   ├── verify-console-config.sh  # Console configuration verification
│   └── README.md
│
├── managed_components/          # ESP-IDF managed components
│   ├── espressif__esp_websocket_client/
│   └── espressif__mdns/
│
├── build/                        # ESP-IDF build output
│   ├── spaxel-firmware.elf      # Linked ELF binary
│   ├── spaxel-firmware.bin      # Merged binary (boot+partition_table+app)
│   ├── project_description.json # Build metadata
│   ├── compile_commands.json    # LSP configuration
│   ├── flasher_args.json        # Flash arguments for esptool
│   └── [CMake build artifacts]
│
├── CMakeLists.txt              # Project-level CMake config
├── partitions.csv              # Flash partition layout (factory+ota_0+ota_1+nvs+otadata)
├── sdkconfig                  # Generated SDK configuration
├── sdkconfig.defaults         # Project-specific defaults (committed)
├── sdkconfig.uart-console      # UART0 console variant
├── sdkconfig.usbjtag         # USB-Serial/JTAG console variant
├── dependencies.lock           # Component dependency lock
├── README.md                  # Firmware documentation
└── CONTRIBUTING.md           # Contribution guidelines
```

### Key Firmware Modules

- **CSI Capture:** `csi.c` — promiscuous mode, CSI callback, binary frame format
- **Transport:** `transport.c` — UART0 + USB-Serial/JTAG dual transport
- **Provisioning:** `provision.c` — Serial JSON parser (10s window on boot)
- **Connectivity:** `wifi.c` — WiFi, mDNS, captive portal AP
- **BLE:** `ble.c` — Passive scan on Core 0, advertisement parsing

---

## `/mothership` — Go Backend

```
mothership/
├── cmd/                             # Entry points
│   ├── mothership/                  # Main mothership binary
│   │   ├── main.go                  # Startup sequencing, subsystem wiring
│   │   ├── dashboard_embed.go      # Dashboard static embed (go:embed)
│   │   ├── mdns_binding.go         # mDNS advertisement
│   │   ├── migrate.go              # Database migration runner
│   │   └── [test files]
│   │
│   └── sim/                        # CSI simulator CLI (spaxel-sim)
│       ├── main.go                  # Simulator entry point
│       ├── generator.go             # Synthetic CSI generation
│       ├── walker.go                # Synthetic walker motion
│       ├── scenario.go              # Test scenario definitions
│       ├── verify.go                # Result verification
│       └── Makefile
│
├── internal/                        # All Go packages
│   │
│   ├── Ingestion & Signal Pipeline
│   │   ├── ingestion/               # WebSocket server, binary frame parsing
│   │   ├── signal/                 # Signal processing pipeline
│   │   │   ├── phase/             # Phase sanitization (unwrap, OLS, residual)
│   │   │   ├── nbvi/              # NBVI subcarrier selection
│   │   │   ├── feature/           # deltaRMS, phase variance, breathing
│   │   │   ├── baseline/          # EMA baseline, diurnal slots
│   │   │   └── processor.go      # Pipeline orchestration
│   │   └── recorder/              # CSI recording buffer (csi_replay.bin)
│   │
│   ├── Localization & Fusion
│   │   ├── localizer/fusion/       # Fusion loop (10 Hz)
│   │   ├── localization/           # Spatial localization
│   │   │   ├── gdop_example.go   # GDOP computation examples
│   │   │   ├── grid.go          # 3D grid accumulation
│   │   │   ├── groundtruth.go   # BLE position estimation
│   │   │   ├── self_improving.go # Weight learning from BLE
│   │   │   ├── spatial_weights.go # Per-link weight storage
│   │   │   └── weightstore.go    # SQLite persistence
│   │   ├── fusion/                 # Fresnel zone accumulation
│   │   ├── tracker/               # UKF blob tracking (6D state)
│   │   └── tracking/              # Legacy tracker (deprecated)
│   │
│   ├── Fleet & Node Management
│   │   ├── fleet/                  # Node registry, role assignment
│   │   │   ├── manager.go         # Node lifecycle
│   │   │   ├── registry.go        # SQLite persistence
│   │   │   ├── optimiser.go      # GDOP-based role optimization
│   │   │   ├── healer.go          # Self-healing on node loss
│   │   │   ├── selfheal.go        # Re-optimization trigger
│   │   │   ├── collision.go       # TX slot collision detection
│   │   │   └── weather.go         # Link quality scoring
│   │   ├── provisioning/          # Node provisioning API
│   │   ├── ota/                   # OTA manager, auto-update
│   │   │   ├── manager.go
│   │   │   ├── autoupdate.go     # Canary deployment
│   │   │   └── autoapi.go         # Firmware serving
│   │   └── apdetector/            # AP auto-detection for passive radar
│   │
│   ├── BLE & Identity
│   │   ├── ble/                    # BLE device registry
│   │   │   ├── registry.go        # Device registry (SQLite)
│   │   │   ├── identity.go        # RSSI triangulation
│   │   │   ├── rotation.go        # Address rotation heuristics
│   │   │   └── label_test.go
│   │   ├── tracker/                # Blob-to-BLE matching
│   │   │   ├── ble_provider.go    # BLE scan aggregation
│   │   │   ├── identity.go        # Person assignment
│   │   │   ├── tracker.go         # Matching logic
│   │   │   └── ukf.go             # Kalman filter
│   │   └── notification/           # Person label resolution
│   │
│   ├── Zones, Portals, Spaces
│   │   ├── zones/                  # Zone manager
│   │   │   ├── manager.go
│   │   │   ├── manager_history.go # Occupancy history
│   │   │   └── manager_migrate.go # Schema migrations
│   │   ├── floorplan/              # Floor plan image + calibration
│   │   ├── volume/                 # Trigger volume shapes (box/cylinder)
│   │   └── portal/                 # Doorway crossing detection
│   │
│   ├── Automation & Triggers
│   │   ├── automation/             # Spatial automation engine
│   │   │   ├── engine.go          # Condition evaluation (10 Hz)
│   │   │   └── identity_fields_test.go
│   │   └── volume_triggers/        # Trigger volume HTTP API
│   │
│   ├── Learning & Prediction
│   │   ├── prediction/             # Presence prediction
│   │   │   ├── predictor.go      # 15-min horizon prediction
│   │   │   ├── model.go          # Probability model (EMA update)
│   │   │   ├── horizon.go        # Time-slot logic
│   │   │   ├── accuracy.go       # Prediction accuracy tracking
│   │   │   └── history.go        # Historical data
│   │   ├── learning/               # Feedback processing
│   │   │   ├── feedback_processor.go # Thumbs up/down handler
│   │   │   ├── feedback_store.go  # SQLite persistence
│   │   │   └── accuracy.go        # Trend computation
│   │   ├── anomaly/                # Anomaly detection (in analytics/)
│   │   ├── patterns/               # Pattern learning (in analytics/)
│   │   └── sleep/                  # Sleep quality monitoring
│   │       ├── analyzer.go        # Breathing + motion detection
│   │       ├── breathing_estimator.go  # FFT-based rate estimation
│   │       ├── breathing_anomaly.go    # Breathing irregularity
│   │       ├── records.go         # Sleep record storage
│   │       └── report.go          # Daily report generation
│   │
│   ├── Recording & Replay
│   │   ├── recording/              # CSI replay buffer
│   │   │   ├── buffer.go          # Append-only file wrapper
│   │   │   ├── compression.go     # Compression analysis
│   │   │   └── benchmark.go       # Performance benchmarks
│   │   ├── replay/                 # Time-travel debugging
│   │   │   ├── engine.go          # Replay session manager
│   │   │   ├── pipeline.go        # Replay pipeline instance
│   │   │   ├── buffer_adapter.go  # CSI replay adapter
│   │   │   ├── session.go         # Session state
│   │   │   ├── worker.go          # Playback worker
│   │   │   ├── store.go           # CSI replay.bin reader
│   │   │   └── types.go
│   │   └── segment/                # Recording segments
│   │
│   ├── Analytics & Flow
│   │   ├── analytics/              # Analytics HTTP handlers
│   │   │   ├── handler.go         # Alert handling, flow, anomaly
│   │   │   ├── anomaly.go         # Anomaly scoring
│   │   │   ├── patterns.go        # Pattern model Welford updates
│   │   │   ├── flow.go            # Crowd flow accumulator
│   │   │   └── alert_handler.go   # Alert chain
│   │   └── flow/                   # (duplicate, use analytics/flow.go)
│   │
│   ├── Dashboard & UI
│   │   ├── dashboard/              # Dashboard server
│   │   │   ├── hub.go             # WebSocket hub (10 Hz feed)
│   │   │   └── server.go          # HTTP static file serving
│   │   ├── explainability/         # Detection explanation
│   │   ├── guidedtroubleshoot/    # Contextual help
│   │   │   ├── discovery.go       # Trigger detection
│   │   │   ├── quality.go         # Quality degradation
│   │   │   └── notifier.go       # Help delivery
│   │   └── timeline/               # Activity timeline
│   │
│   ├── Notifications & Messaging
│   │   ├── notifications/          # Notification manager
│   │   │   ├── manager.go
│   │   │   ├── ntfy.go            # Ntfy channel
│   │   │   ├── pushover.go        # Pushover channel
│   │   │   └── webhook.go          # Webhook channel
│   │   ├── notify/                 # Notification service
│   │   │   ├── service.go         # Sync notification dispatch
│   │   │   └── service_enhanced.go # Image rendering, batching
│   │   ├── render/                 # Floor plan thumbnail rendering
│   │   └── briefing/               # Morning briefing
│   │       ├── briefing.go        # Briefing generation
│   │       ├── scheduler.go       # Daily scheduling
│   │       └── dashboard_adapter.go
│   │
│   ├── MQTT & Integration
│   │   ├── mqtt/                   # MQTT client
│   │   │   ├── client.go          # Eclipse Paho client
│   │   │   └── publisher.go       # HA auto-discovery
│   │   └── webhook/                # Webhook publisher
│   │
│   ├── API Layer
│   │   ├── api/                    # REST API handlers
│   │   │   ├── [*_test.go]        # Per-endpoint tests
│   │   │   ├── status.go          # GET /api/status
│   │   │   ├── settings.go        # GET/PATCH /api/settings
│   │   │   ├── network_settings.go # WiFi credential API
│   │   │   ├── zones.go           # Zone CRUD
│   │   │   ├── triggers.go        # Trigger CRUD
│   │   │   ├── baseline.go        # Baseline snapshots
│   │   │   ├── events.go          # Activity timeline
│   │   │   ├── replay.go          # Time-travel API
│   │   │   ├── analytics.go       # Crowd flow API
│   │   │   ├── prediction.go      # Prediction API
│   │   │   ├── sleep.go           # Sleep reports
│   │   │   ├── notification_settings.go
│   │   │   ├── notifications.go   # Notification channels
│   │   │   ├── security.go        # Security mode API
│   │   │   ├── integrations.go    # HA integration status
│   │   │   ├── backup.go          # Backup/export
│   │   │   ├── briefing.go       # Morning briefing
│   │   │   ├── feedback.go        # Thumbs up/down
│   │   │   ├── simulator.go       # Simulator control
│   │   │   └── utils.go           # Common utilities
│   │   └── auth/                   # Authentication
│   │       ├── handler.go        # Session cookie, HMAC token
│   │       └── token_fuzz_test.go
│   │
│   ├── System & Infrastructure
│   │   ├── config/                 # Environment config loading
│   │   ├── db/                     # SQLite database
│   │   │   ├── db.go              # Connection, WAL mode
│   │   │   ├── migrate.go         # Migration runner
│   │   │   ├── migrations.go      # Schema definitions
│   │   │   └── fix_migration.sed
│   │   ├── health/                 # Health check (GET /healthz)
│   │   ├── startup/                # Startup sequencing (7 phases)
│   │   ├── shutdown/               # Graceful shutdown (SIGTERM)
│   │   ├── diskspace/              # Disk space monitoring
│   │   ├── doctor/                 # System diagnostics
│   │   ├── loadshed/               # Load shedding under pressure
│   │   └── eventbus/               # In-memory event bus
│   │
│   ├── Diagnostics & Monitoring
│   │   ├── diagnostics/            # Link weather diagnostics
│   │   │   ├── linkweather.go     # Per-link health metrics
│   │   │   └── reposition.go      # Repositioning advice
│   │   ├── falldetect/             # Fall detection
│   │   ├── explainability/         # Detection explainability
│   │   └── help/                   # Help article monitor
│   │
│   ├── Virtual & Testing
│   │   ├── simulator/              # CSI simulator for testing
│   │   │   ├── engine.go          # Simulator orchestration
│   │   │   ├── node.go           # Virtual node
│   │   │   ├── walker.go         # Synthetic walker
│   │   │   ├── physics.go         # Signal propagation model
│   │   │   ├── gdop.go           # GDOP computation
│   │   │   ├── space.go          # Room geometry
│   │   │   ├── session.go        # Simulator session
│   │   │   ├── virtual_state.go  # Virtual blob/node state
│   │   │   ├── registry_bridge.go # Mothership registry bridge
│   │   │   └── types.go
│   │   └── oui/                    # OUI lookup table
│   │       ├── gen.go             # Generate from IEEE oui.txt
│   │       ├── gen_data.go       # Generation script
│   │       ├── oui_data.go       # Embedded OUI map
│   │       └── oui.go             # Lookup function
│   │
│   └── Events & Storage
│       ├── events/                 # Event storage and FTS5
│       │   ├── events.go          # Event types
│       │   ├── storage.go         # SQLite persistence
│       │   ├── bus.go             # Publish/subscribe
│       │   └── types.go
│       └── recorder/              # Recording segment management
│           ├── manager.go        # Segment lifecycle
│           └── segment.go       # Segment metadata
│
├── test/                           # Mothership tests
│   ├── acceptance/                # Simulator-based acceptance tests
│   │   ├── acceptance_test.go    # Test runner
│   │   ├── as1_setup_test.go    # First-run setup
│   │   ├── as2_walking_test.go   # Walking detection
│   │   ├── as3_fall_test.go      # Fall detection
│   │   ├── as4_ble_test.go      # BLE identity
│   │   ├── as5_ota_test.go      # OTA rollback
│   │   ├── as6_replay_test.go   # Time-travel replay
│   │   └── as7_auth_reject_test.go # Token rejection
│   └── wifi_credential_*_test.go  # WiFi credential e2e tests
│
└── tests/e2e/                     # Shell-based e2e tests
    └── run.sh

54 internal packages, 314+ Go files
```

---

## `/dashboard` — Web UI (Vanilla JS + Three.js)

```
dashboard/
├── index.html                   # Expert mode (3D view, default)
├── live.html                   # Live view alias
├── simple.html                 # Simple mode (cards, no 3D)
├── ambient.html                # Ambient mode (wall-mount display)
├── setup.html                  # Setup / calibration wizard
├── fleet.html                  # Fleet status table
├── integrations.html           # HA integration status
├── simulator.html              # Simulator interface
├── test-transformcontrols.html # TransformControls testing

├── css/                         # Stylesheets (28 files)
│   ├── layout.css              # Main layout
│   ├── scene.css               # 3D scene styling
│   ├── panels.css              # UI panels
│   ├── timeline.css            # Activity timeline
│   ├── command-palette.css    # Ctrl+K command palette
│   ├── explainability.css      # "Why?" overlay
│   ├── quick-actions.css      # Context menus
│   ├── replay.css              # Time-travel UI
│   ├── anomaly.css             # Anomaly visualization
│   ├── briefing.css            # Morning briefing card
│   ├── sleep.css               # Sleep reports
│   ├── simple.css              # Simple mode cards
│   ├── ambient.css             # Ambient mode
│   ├── wizard.css              # Onboarding wizard
│   ├── fleet-page.css          # Fleet table
│   ├── integrations.css        # HA integration
│   ├── notifications.css       # Notification settings
│   ├── settings-panel.css      # Settings panel
│   ├── troubleshoot.css        # Guided troubleshooting
│   ├── security-panel.css      # Security mode
│   ├── apdetection.css         # AP detection UI
│   ├── ble-panel.css           # BLE device panel
│   ├── floorplan.css           # Floor plan editor
│   ├── fresnel.css             # Fresnel zone debug
│   ├── home.css                # Home cards
│   ├── linkhealth.css          # Link quality viz
│   ├── crowdflow.css           # Crowd flow arrows
│   ├── tokens.css              # Token input styling
│   └── [helper scripts]

├── js/                          # JavaScript modules (70+ files)
│   ├── app.js                   # App entry point
│   ├── router.js               # Hash-based router
│   ├── state.js                 # Global state store
│   ├── websocket.js            # WebSocket connection manager
│   ├── auth.js                  # PIN authentication
│   ├── viz3d.js                # Three.js 3D scene
│   │   ├── scene setup (nodes, blobs, zones)
│   │   ├── OrbitControls
│   │   ├── blob rendering (humanoid figures)
│   │   └── Fresnel zone debug overlay
│   ├── fresnel.js              # Fresnel zone computation
│   ├── floorplan-setup.js      # Floor plan editor
│   ├── zone-editor.js          # Zone CRUD
│   ├── portal.js               # Portal drawing
│   ├── placement.js            # Node positioning
│   ├── volume-editor.js        # Trigger volume editor
│   ├── onboard.js              # Onboarding wizard
│   ├── fleet.js                # Fleet status panel
│   ├── fleet-page.js           # Fleet page specific
│   ├── ble-panel.js            # BLE device registry
│   ├── blob-identity.js        # BLE-to-blob matching
│   ├── automations.js          # Spatial automation builder
│   ├── automation-builder.js   # Automation engine
│   ├── proactive.js            # Proactive alerts
│   ├── anomaly.js              # Anomaly visualization
│   ├── timeline.js             # Activity timeline component
│   ├── sidebar-timeline.js    # Timeline sidebar
│   ├── explainability.js      # "Why?" explanation
│   ├── explain.js              # Explanation renderer
│   ├── feedback.js             # Thumbs up/down
│   ├── replay.js                # Time-travel debugging
│   ├── diurnal-chart.js        # Diurnal baseline chart
│   ├── sleep.js                # Sleep quality visualization
│   ├── briefing.js             # Morning briefing
│   ├── notifications.js        # Notification settings
│   ├── settings-panel.js       # Settings editor
│   ├── security-panel.js       # Security mode
│   ├── integrations.js         # HA integration status
│   ├── simulate.js             # Simulator interface
│   ├── simple-mode.js          # Simple mode logic
│   ├── ambient.js              # Ambient mode renderer
│   ├── ambient_renderer.js     # Ambient 2D canvas drawing
│   ├── ambient_briefing.js     # Ambient briefing display
│   ├── command-palette.js      # Ctrl+K command palette
│   ├── quick-actions.js        # Context menu handler
│   ├── troubleshoot.js         # Guided troubleshooting
│   ├── guided-help.js          # Help article delivery
│   ├── help.js                 # Help system
│   ├── controls.js             # View preset buttons
│   ├── layers.js               # Layer toggles
│   ├── tooltip.js              # Tooltip system
│   ├── tooltips.js             # Tooltip content
│   ├── linkhealth.js           # Link quality visualization
│   ├── crowdflow.js            # Crowd flow arrows
│   ├── zone-lookup.js          # Zone lookup helper
│   ├── ota.js                  # OTA control
│   ├── [test files]            # Jest + Playwright tests
│   └── [profiling files]       # Leak detection profiles

├── static/                      # Static assets (copied to container)
│   ├── css/                     # CSS copies
│   ├── icons/                   # Icon images
│   └── js/                      # JS copies

├── types/                       # TypeScript definitions
│   ├── spaxel.d.ts             # API types
│   └── blob-identity.check.ts  # Type checking

├── tests/                       # Playwright accessibility tests
│   ├── a11y.spec.js
│   ├── a11y-dashboard.spec.js
│   ├── a11y-onboarding.spec.js
│   └── accessibility/

├── node_modules/               # npm dependencies (300+ packages)
├── test-results/               # Jest test results

├── package.json                 # npm dependencies
├── package-lock.json           # npm lock file
├── tsconfig.json              # TypeScript config
├── jest.config.js             # Jest config
├── playwright.config.js        # Playwright config
├── manifest.json              # PWA manifest
├── sw.js                      # Service Worker
├── generate-icons.js          # Icon generation script
├── README.md                  # Dashboard documentation
├── leak-detection-report.json  # Leak detection results
└── leak-test-full-lifecycle.json
```

### Dashboard Entry Points
- `/` → Expert 3D mode (default)
- `/simple.html` → Simple card mode
- `/ambient.html` → Wall-mount ambient mode
- `/setup.html` → Onboarding wizard
- `/fleet.html` → Fleet status table
- `/simulator.html` → Simulator interface

---

## `/docs` — Documentation

```
docs/
├── plan/
│   ├── plan.md                  # Complete implementation plan
│   └── COMPRESSION_BENCHMARKS.md
│
├── research/
│   ├── 01-csi-fundamentals.md
│   ├── 02-physics.md
│   ├── 03-algorithms.md
│   ├── 04-signal-processing.md
│   ├── 05-mesh-topology.md
│   ├── 06-accuracy-and-limits.md
│   ├── 07-literature.md
│   ├── papers/                 # Research paper references
│   ├── ota-race-condition-quick-test.md
│   └── ota-wifi-reconnection-race-condition-testing.md
│
├── notes/                       # Technical notes
│   ├── adr-010-error-handling-patterns.md
│   ├── adr-011-esp-error-check-vs-graceful-error-handling.md
│   ├── ambient-traffic-measurement.md
│   ├── auto-update-version-selection-investigation.md
│   ├── ci-doc-only-push-path-filter.md
│   ├── ci-e2e-template-runsh-audit.md
│   ├── ci-test-sim-reference-map.md
│   ├── error-handling-patterns.md
│   ├── esp32-ota-and-reconnection-handoff.md
│   ├── firmware-build-flash-monitor-runbook.md
│   ├── firmware-host-test-approach.md
│   ├── fusion-setnodeposition-wiring-verified.md
│   ├── mdns-override.md
│   ├── ota-security-hardening-2026-08-15.md
│   ├── ota-wifi-race-investigation.md
│   ├── ota-wifi-reconnection-race-summary.md
│   ├── recovery-mechanisms.md
│   ├── simulation-testing.md
│   ├── token-reject-root-cause.md
│   ├── token-supply-path-dead-confirm.md
│   ├── token-validator-wiring-confirm.md
│   ├── ux-visualization.md
│   ├── wifi-restart-race-test-plan.md
│   ├── 3d-blob-visualization-architecture.md
│   ├── bf-15oi-runtime-capture/ (runtime capture)
│   ├── bf-19ufa-blob-creation-trace.md
│   ├── bf-2gmx-runtime-capture/
│   └── bf-4do5y-runtime-capture/
│
├── deployment/
│   ├── environment-variables.md
│   ├── migration-guide.md
│   └── wifi-configuration.md
│
├── examples/
│   ├── GDOP_USAGE_GUIDE.md
│   ├── gdop_usage_examples.go
│   └── gdop-usage-example-enhanced.go
│
├── tests/
│   └── manual-ota-during-wifi-reconnect-test.md
│
├── gdop-computation-functions.md
├── gdop-function-analysis.md
├── gdop-function-signature.md
├── gdop-usage-example.go
├── gdop-usage-example-enhanced.go
├── SYSTEM_CATALOG.md
├── ci-accessibility-integration.md
├── ci-benchmark-integration.md
└── wifi-credential-provisioning-flow.md
```

---

## `/cmd` — Simulator CLI

```
cmd/
└── sim/
    ├── main.go              # CLI entry point
    ├── generator.go         # CSI frame generator
    ├── walker.go            # Walker motion generator
    ├── scenario.go          # Test scenarios
    ├── verify.go           # Result verification
    ├── Makefile             # Build script
    └── README.md            # Documentation
```

---

## `/scripts` — Utility Scripts

```
scripts/
├── flash-esp32s3.sh               # ESP32 flashing helper
├── provision_esp32.py            # Provisioning helper
├── run-sim-local.sh             # Local simulator run
├── run-sim-identity.sh          # Identity fixture test
├── run-sim-ble-match.sh         # BLE match test
├── run-sim-ble-fixture.sh       # BLE fixture test
├── run-sim-dashboard-console.sh # Console integration test
└── capture-dashboard-console.mjs  # Console capture
```

---

## `/test` — Acceptance Tests

```
test/
└── acceptance/
    ├── acceptance_test.go      # Test runner
    ├── as1_setup_test.go      # Scenario: First-time setup
    ├── as2_walking_test.go    # Scenario: Walking detection
    ├── as3_fall_test.go       # Scenario: Fall alert
    ├── as4_ble_test.go        # Scenario: BLE identity
    ├── as5_ota_test.go       # Scenario: OTA update
    ├── as6_replay_test.go    # Scenario: Time-travel replay
    └── as7_auth_reject_test.go # Scenario: Token rejection
```

---

## `/tests` — E2E Tests

```
tests/
└── e2e/
    └── run.sh                   # Shell-based e2e harness
```

---

## `/notes` — Investigation Notes

```
notes/
├── bf-*.md                     # NEEDLE bead notes (60+ files)
│   ├── bf-4bhd.md
│   ├── bf-5cgc-handoff.md
│   ├── bf-3aij-pattern.md
│   ├── bf-3y9r.md
│   ├── bf-26ta-findings.md
│   └── [many more...]
│
├── ble-identity-fixture.md     # BLE identity investigation
├── blob_observation.md          # Blob observation notes
├── blob-identity-diagnosis.md   # Blob identity diagnosis
├── hardware-free-runtime.md     # Hardware-free testing notes
```

---

## `/.beads` — NEEDLE Workspace State

```
.beads/
├── config.json                 # Bead backend configuration
├── beads.db                    # SQLite bead database
├── events.jsonl                # Event log
├── heartbeats.jsonl            # Heartbeat log
│
├── checkpoint/                 # Git-tracked checkpoint
│   ├── current.json           # Current checkpoint state
│   ├── previous.json          # Previous checkpoint
│   ├── forensic.jsonl        # Forensic log
│   ├── manifests/            # Checkpoint manifests
│   └── objects/              # Checkpoint objects (gen-*.jsonl)
│
├── receipts/                   # Bead receipts
├── recovery/                   # Recovery records
│   └── run-20260804T020503-394/
│
└── traces/                     # Trace logs (73 files)
    ├── spaxel-069d866b/
    ├── spaxel-075ef1c4/
    ├── spaxel-9b4087cc/       # Current bead
    └── [70 more trace dirs...]
```

---

## Build-Relevant Path Patterns

This catalog serves as the reference for determining which paths trigger builds:

### Firmware Build Triggers
```
firmware/main/*.c
firmware/main/*.h
firmware/CMakeLists.txt
firmware/partitions.csv
firmware/sdkconfig.defaults
firmware/test/*.c
firmware/test/*.h
firmware/scripts/*.sh
VERSION
```

### Mothership Build Triggers
```
mothership/cmd/**/*.go
mothership/internal/**/*.go
mothership/test/**/*.go
mothership/**/go.mod
mothership/**/go.sum
VERSION
```

### Dashboard Build Triggers
```
dashboard/*.html
dashboard/css/*.css
dashboard/js/*.js
dashboard/static/**
dashboard/types/**
dashboard/package.json
dashboard/tsconfig.json
VERSION
```

### Documentation Triggers
```
docs/**/*.md
docs/**/*.go
README.md
PROGRESS.md
```

### Configuration Triggers
```
docker-compose.yml
Dockerfile
.golangci.yml
.jest.config.js
.playwright.config.js
sdkconfig.defaults
```

---

## Statistics

| Component | Directories | Files | Key Languages |
|-----------|-------------|-------|---------------|
| Root | - | 20 | YAML, Shell, Markdown |
| firmware | 5 | 35 | C, CMake, Shell |
| mothership | 58 | 314+ | Go, SQL |
| dashboard | 7 | 138+ | JS, HTML, CSS |
| docs | 11 | 52 | Markdown, Go (examples) |
| cmd/sim | 1 | 8 | Go, Makefile |
| test/acceptance | 1 | 8 | Go |
| scripts | 1 | 8 | Shell, Python, JS |
| .beads | 6 | 80+ | JSON, JSONL |

**Total:** 90+ directories, 650+ files

---

## Module Boundaries (Go Workspace)

The repository uses a Go workspace with 3 modules:
1. **`mothership/`** — Main backend module (go.mod in mothership/)
2. **`cmd/sim/`** — Simulator CLI module (go.mod in cmd/sim/)
3. **`test/acceptance/`** — Acceptance test module (go.mod in test/acceptance/)

Each module has its own `go.mod` and is versioned independently from the monorepo VERSION.

---

**End of Catalog**
