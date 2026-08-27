# Build Paths for Spaxel

This document identifies which file paths contain substantive code that should trigger spaxel rebuilds and deployments. Changes to these paths require rebuilding and/or redeploying the application.

## Core Application Code

### ESP32-S3 Firmware (`firmware/`)
**Trigger:** Firmware rebuild + OTA deployment

All C source and build configuration files under `firmware/` that are compiled into the ESP32-S3 firmware binary:

```
firmware/
├── main/                    # Primary application code
│   ├── *.c                  # C source files (main.c, csi.c, wifi.c, etc.)
│   ├── *.h                  # C header files
│   └── CMakeLists.txt       # Build configuration
├── CMakeLists.txt           # Top-level firmware build
├── partitions.csv            # Flash partition table
├── sdkconfig.defaults        # ESP-IDF project configuration
└── managed_components/      # ESP-IDF component dependencies
```

**Note:** Changes to `firmware/test/` do NOT trigger production builds (test-only).

### Go Backend (`mothership/`)
**Trigger:** Docker image rebuild + container restart

All Go source code and module files under `mothership/`:

```
mothership/
├── cmd/
│   ├── mothership/          # Main application entry point
│   │   └── *.go             # All Go files
│   └── sim/                 # Simulator CLI
│       └── *.go
├── internal/                # All internal packages
│   ├── */                   # Each subdirectory contains Go packages
│   │   └── *.go
│   └── */                   # (apdetector, automation, db, fleet, etc.)
└── go.mod                   # Go module definition
    go.sum                   # Dependency lock file
```

**Note:** Test files (`*_test.go`) do NOT trigger production builds.

### Frontend Dashboard (`dashboard/`)
**Trigger:** Docker image rebuild (embedded via go:embed)

All frontend source files that get embedded into the mothership binary:

```
dashboard/
├── *.html                   # HTML pages (index.html, live.html, etc.)
├── js/                      # JavaScript application code
│   └── *.js
├── css/                     # Stylesheets
│   └── *.css
├── static/                  # Static assets (images, etc.)
│   └── *
├── package.json             # NPM dependencies
└── tsconfig.json            # TypeScript configuration
```

**Note:** `dashboard/node_modules/` and `dashboard/test-results/` do NOT trigger builds (generated directories).

## Build Configuration

### Container Build
**Trigger:** Docker image rebuild

```
Dockerfile                   # Multi-stage build definition
```

### Deployment Configuration
**Trigger:** Deployment restart (Kubernetes deployment update)

```
docker-compose.yml           # Local development deployment
```

**Note:** Production uses Kubernetes manifests from `jedarden/declarative-config` (external repository).

## Version File

### Version Information
**Trigger:** Docker image rebuild + version tagging

```
VERSION                      # Single source of truth for release version
```

Changes to `VERSION` affect:
- Docker image tags
- Firmware version reporting
- API version responses

## Go Workspace Configuration

### Module Configuration
**Trigger:** Docker image rebuild (affects Go build)

```
go.work                      # Go workspace configuration
go.work.sum                  # Workspace dependency lock
```

## Scripts and Utilities

### Build and Test Scripts
**Trigger:** Varies by script purpose

```
scripts/
├── provision_esp32.py       # Used during onboarding (no build trigger)
└── measure_csi_rate.py      # Diagnostic tool (no build trigger)
```

**Note:** Most scripts are utilities and do NOT trigger production builds.

## Documentation (NO Build Trigger)

### Documentation Files
These files do NOT trigger builds:

```
*.md                         # All Markdown documentation
docs/                        # Documentation directory
notes/                       # Development notes
README.md                    # Project README
PROGRESS.md                  # Implementation progress
SYSTEM_CATALOG.md           # System catalog
```

## Testing Code (NO Build Trigger)

### Test Files
These files do NOT trigger production builds:

```
**/*_test.go                 # Go test files
firmware/test/               # Firmware test suite
testdata/                    # Test data files
test/                        # Integration tests
tests/                       # E2E tests
dashboard/tests/             # Frontend tests
dashboard/test-results/      # Test output
```

## Generated Directories (NO Build Trigger)

### Build Artifacts
These directories are generated during build and should NOT trigger builds:

```
firmware/build/              # ESP-IDF build output
firmware/managed_components/ # ESP-IDF component cache
dashboard/node_modules/      # NPM dependencies
```

## Summary Matrix

| Path | Component | Build Trigger | Deployment Trigger |
|------|-----------|---------------|-------------------|
| `firmware/main/*.c` | ESP32 Firmware | ✅ Firmware rebuild | ✅ OTA rollout |
| `firmware/main/*.h` | ESP32 Firmware | ✅ Firmware rebuild | ✅ OTA rollout |
| `firmware/CMakeLists.txt` | ESP32 Firmware | ✅ Firmware rebuild | ✅ OTA rollout |
| `firmware/partitions.csv` | ESP32 Firmware | ✅ Firmware rebuild | ✅ OTA rollout |
| `firmware/sdkconfig.defaults` | ESP32 Firmware | ✅ Firmware rebuild | ✅ OTA rollout |
| `mothership/cmd/mothership/*.go` | Backend | ✅ Docker rebuild | ✅ Container restart |
| `mothership/internal/**/*.go` | Backend | ✅ Docker rebuild | ✅ Container restart |
| `mothership/go.mod` | Backend | ✅ Docker rebuild | ✅ Container restart |
| `dashboard/*.html` | Frontend | ✅ Docker rebuild | ✅ Container restart |
| `dashboard/js/*.js` | Frontend | ✅ Docker rebuild | ✅ Container restart |
| `dashboard/css/*.css` | Frontend | ✅ Docker rebuild | ✅ Container restart |
| `Dockerfile` | Build | ✅ Docker rebuild | ✅ Container restart |
| `VERSION` | All | ✅ Docker rebuild | ✅ Container restart |
| `docker-compose.yml` | Deploy | ❌ No rebuild | ✅ Container restart |
| `*.md` | Docs | ❌ No trigger | ❌ No trigger |
| `*_test.go` | Tests | ❌ No trigger | ❌ No Trigger |
| `firmware/test/` | Tests | ❌ No trigger | ❌ No trigger |

## Integration with Path Filters

This list feeds directly into path filter implementation for CI/CD systems. For example:

**Example GitHub Actions filter:**
```yaml
paths:
  - 'firmware/main/**'
  - 'firmware/CMakeLists.txt'
  - 'firmware/partitions.csv'
  - 'firmware/sdkconfig.defaults'
  - 'mothership/cmd/mothership/**'
  - 'mothership/internal/**'
  - 'mothership/go.mod'
  - 'dashboard/**'
  - 'Dockerfile'
  - 'VERSION'
```

**Excluded paths (documentation, tests, generated):**
```yaml
paths-ignore:
  - '**/*.md'
  - '**_test.go'
  - 'firmware/test/**'
  - 'testdata/**'
  - 'test/**'
  - 'tests/**'
  - 'dashboard/test-results/**'
  - 'dashboard/node_modules/**'
  - 'firmware/build/**'
```

## Notes

1. **Firmware builds** require the ESP-IDF toolchain and produce `spaxel-firmware.bin`
2. **Docker builds** produce the multi-arch mothership image with embedded dashboard
3. **Deployment** typically refers to Kubernetes rolling updates in production
4. **Local development** uses `docker-compose.yml` for simpler workflow
