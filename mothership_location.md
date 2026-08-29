# Mothership Repository Location

## Summary
The mothership codebase is located within the spaxel workspace at:

**Base Path**: `/home/coding/spaxel/mothership/`

## Directory Structure

### Spaxel Workspace (Root: `/home/coding/spaxel/`)
```
spaxel/
├── mothership/        # Go backend (mothership) - PRIMARY CODEBASE
├── firmware/          # ESP-IDF C firmware for ESP32-S3
├── dashboard/         # Vanilla JS + Three.js frontend
├── cmd/              # Additional tools and simulators
├── docs/             # Documentation (plan, research, notes)
├── test/             # Cross-cutting acceptance tests
├── data/             # Runtime data directory
├── scripts/          # Utility scripts
└── Dockerfile        # Multi-stage build configuration
```

### Mothership Module Structure (`/home/coding/spaxel/mothership/`)
```
mothership/
├── cmd/
│   ├── mothership/    # Main application entrypoint (main.go)
│   └── sim/          # CSI simulator CLI
├── internal/
│   ├── ingestion/     # WebSocket server, binary frame parsing
│   ├── pipeline/      # Signal processing (phase, NBVI, feature, baseline)
│   ├── localizer/     # Fusion, Fresnel localization, UKF, GDOP
│   ├── fleet/         # Node registry, role assignment
│   ├── ble/           # BLE identity matching
│   ├── portal/        # Zone crossing detection
│   ├── replay/        # CSI replay buffer management
│   ├── anomaly/       # Anomaly detection patterns
│   ├── predict/       # Presence prediction
│   ├── sleep/         # Sleep quality monitoring
│   ├── flow/          # Crowd flow visualization
│   ├── notify/        # Notification rendering
│   ├── mqtt/          # MQTT client integration
│   ├── auth/          # Authentication (HMAC, bcrypt, sessions)
│   ├── oui/           # OUI lookup table
│   ├── db/            # SQLite database and migrations
│   ├── config/        # Environment variable parsing
│   └── [50+ other internal packages]
├── go.mod             # Go module definition (github.com/spaxel/mothership)
├── go.sum             # Go module dependencies
├── mothership         # Compiled binary
├── sim                # Simulator binary
└── test/              # Mothership-specific tests
```

## Key Information

- **Go Module**: `github.com/spaxel/mothership`
- **Go Version**: 1.25.0
- **Entry Point**: `cmd/mothership/main.go`
- **Architecture**: Single Docker container ("mothership") managing ESP32-S3 fleet
- **Language**: Pure Go (no CGO, uses `modernc.org/sqlite`)
- **Primary Purpose**: CSI ingestion, signal processing, localization, fleet management, dashboard server

## Related Components

- **Firmware**: ESP-IDF C code at `/home/coding/spaxel/firmware/`
- **Dashboard**: Static assets at `/home/coding/spaxel/dashboard/` (embedded via `go:embed`)
- **Simulator**: `cmd/sim/` for hardware-free testing

## Notes
- Mothership is a standalone Go module within the larger spaxel workspace
- The workspace uses Go workspaces (`go.work`) to coordinate multiple modules
- All components are versioned together using the shared `VERSION` file at repository root
