# Mothership Dashboard Code and Configuration Files

## Overview

The Spaxel Mothership dashboard is a web-based user interface served by the Go backend. It provides real-time visualization of WiFi CSI-based indoor positioning system, including 3D scene rendering, fleet management, and system configuration.

## Main Directory Structure

### Dashboard Frontend (`/dashboard/`)

**Location:** `/home/coding/spaxel/dashboard/`

This directory contains all frontend code for the Mothership dashboard:

#### HTML Entry Points
- **`index.html`** - Home page with status overview cards
- **`live.html`** - Main 3D live view with Three.js rendering
- **`simple.html`** - Simplified card-based interface for non-technical users
- **`ambient.html`** - Ambient mode for wall-mounted tablets (simplified top-down view)
- **`fleet.html`** - Fleet status and device management
- **`setup.html`** - Space definition and node placement
- **`integrations.html`** - External service integrations (Home Assistant, MQTT)
- **`simulator.html`** - Pre-deployment simulator tool
- **`test-transformcontrols.html`** - Testing page for transform controls

#### JavaScript Modules (`/dashboard/js/`)

**Core Application Files:**
- **`app.js`** - Main application entry point and initialization
- **`router.js`** - Client-side routing for SPA navigation
- **`state.js`** - Global state management
- **`websocket.js`** - WebSocket connection and message handling
- **`auth.js`** - Authentication (PIN handling, session management)

#### 3D Visualization and Scene Rendering
- **`viz3d.js`** - Three.js 3D scene setup and rendering
- **`scene.js`** - 3D scene object management (nodes, links, zones, blobs)
- **`controls.js`** - Camera controls and orbit manipulation
- **`layers.js`** - Layer visibility management (links, Fresnel zones, trails)
- **`fxaa.js`** - Anti-aliasing effect (FXAA implementation)

#### CSI Processing and Features
- **`fresnel.js`** - Fresnel zone visualization and computation
- **`explainability.js`** - Detection explanation ("Why is this here?")
- **`explain.js`** - Explainability UI components
- **`replay.js`** - Time-travel debugging and CSI replay

#### Zone and Spatial Management
- **`zone-editor.js`** - Zone creation and editing
- **`zone-lookup.js`** - Zone membership queries
- **`portal.js`** - Portal (doorway) management
- **`floorplan-setup.js`** - Floor plan image upload and calibration
- **`placement.js`** - Node placement with coverage painting (GDOP)
- **`volume-editor.js`** - 3D trigger volume creation and editing

#### Fleet and Device Management
- **`fleet.js`** - Fleet management core logic
- **`fleet-page.js`** - Fleet status UI components
- **`onboard.js`** - Device onboarding wizard
- **`ota.js`** - Over-the-air firmware update interface
- **`ble-panel.js`** - BLE device registry and identity management
- **`blob-identity.js`** - BLE-to-blob identity matching

#### Automation and Triggers
- **`automation-builder.js`** - Visual automation builder
- **`automations.js`** - Automation management UI
- **`quick-actions.js`** - Context menu actions
- **`security-panel.js`** - Security mode controls

#### Analytics and Monitoring
- **`timeline.js`** - Activity timeline component
- **`sidebar-timeline.js`** - Timeline sidebar implementation
- **`sleep.js`** - Sleep quality monitoring
- **`diurnal-chart.js`** - Diurnal baseline visualization
- **`crowdflow.js`** - Crowd flow visualization
- **`anomaly.js`** - Anomaly detection display

#### Settings and Configuration
- **`settings-panel.js`** - System settings interface
- **`notifications.js`** - Notification channel configuration
- **`feedback.js`** - User feedback collection (thumbs up/down)
- **`proactive.js`** - Proactive system suggestions

#### Specialized Features
- **`ambient.js`** - Ambient mode renderer
- **`ambient_renderer.js`** - Ambient mode Canvas 2D rendering
- **`ambient_briefing.js`** - Morning briefing in ambient mode
- **`briefing.js`** - Morning briefing generation
- **`help.js`** - Help system and articles
- **`command-palette.js`** - Ctrl+K command palette
- **`guided-help.js`** - Guided troubleshooting system
- **`troubleshoot.js`** - Troubleshooting workflows
- **`simulate.js`** - Simulator integration
- **`linkhealth.js`** - Link health diagnostics
- **`accuracy.js`** - Accuracy tracking and trends

#### Testing and Development
- **`profile-suite.js`** - Performance profiling suite
- **`run-profiled-tests.js`** - Test runner with profiling
- **`testProfiler.js`** - Test profiling utilities
- **`esptool-bundle.js`** - ESP tool JavaScript bundle for device provisioning

#### Type Definitions
- **`types/spaxel.d.ts`** - TypeScript type definitions for Spaxel API
- **`types/blob-identity.check.ts`** - Type checking for blob identity

### CSS Styling (`/dashboard/css/`)

**Core Styling:**
- **`tokens.css`** - CSS design tokens (colors, spacing, typography)
- **`layout.css`** - Layout structure and app shell
- **`home.css`** - Home page specific styles
- **`expert.css`** - Expert mode styles
- **`simple.css`** - Simple mode styles
- **`scene.css`** - 3D scene specific styles

**Feature-Specific Styles:**
- **`floorplan.css`** - Floor plan editor styles
- **`fleet-page.css`** - Fleet management UI
- **`panels.css`** - Panel component styles
- **`timeline.css`** - Timeline component
- **`notifications.css`** - Notification display
- **`security.css`** - Security mode styling
- **`sleep.css`** - Sleep analysis display
- **`ambient.css`** - Ambient mode styling
- **`replay.css`** - Time-travel replay UI
- **`explainability.css`** - Detection explanation overlay
- **`command-palette.css`** - Command palette UI
- **`quick-actions.css`** - Context menu styles
- **`guided-help.css`** - Guided troubleshooting UI
- **`troubleshoot.css`** - Troubleshooting page styles

**Specialized Features:**
- **`apdetection.css`** - Access point detection
- **`ble-panel.css`** - BLE device registry panel
- **`anomaly.css`** - Anomaly detection display
- **`briefing.css`** - Morning briefing card
- **`simulator.css`** - Simulator interface
- **`wizard.css`** - Onboarding wizard styles

**Static Assets (`/dashboard/static/`):**
- **`icons/`** - PWA icons (various sizes: 72x72 to 512x512)
- **`css/mobile.css`** - Mobile-specific styles
- **`js/`** - Additional JavaScript files

### Testing (`/dashboard/tests/`)

- **`a11y-dashboard.spec.js`** - Playwright accessibility tests for dashboard
- **`a11y-onboarding.spec.js`** - Playwright accessibility tests for onboarding
- **`a11y.spec.js`** - General accessibility tests
- **`accessibility/`** - Accessibility test utilities

## Go Backend Integration

### Mothership Command (`/mothership/cmd/mothership/`)

**Location:** `/home/coding/spaxel/mothership/cmd/mothership/`

#### Main Application Files
- **`main.go`** - Application entry point, HTTP server setup, dashboard serving
- **`dashboard_embed.go`** - Dashboard embedding via `go:embed` (production builds)
- **`dashboard_static_test.go`** - Dashboard static file serving tests
- **`migrate.go`** - Database migration handling
- **`mdns_binding.go`** - mDNS server binding

### Dashboard Server Package (`/mothership/internal/dashboard/`)

**Location:** `/home/coding/spaxel/mothership/internal/dashboard/`

- **`server.go`** - WebSocket server for `/ws/dashboard` endpoint
- **`hub.go`** - WebSocket connection hub and message broadcasting
- **`hub_test.go`** - Hub package tests

### HTTP API Layer (`/mothership/internal/api/`)

**Location:** `/home/coding/spaxel/mothership/internal/api/`

Dashboard-related API handlers:
- **`status.go`** - System status endpoint
- **`settings.go`** - System settings management
- **`network_settings.go`** - Network credentials configuration
- **`events.go`** - Events API for timeline
- **`localization.go`** - Blob/position API
- **`zones.go`** - Zone management API
- **`triggers.go`** - Spatial automation triggers
- **`volume_triggers.go`** - Trigger volume management
- **`integrations.go`** - External integrations API
- **`notifications.go`** - Notification settings
- **`notification_settings.go`** - Notification channel configuration
- **`backup.go`** - System backup/restore
- **`replay.go`** - CSI replay and time-travel API
- **`simulator.go`** - Simulator API
- **`analytics.go`** - Analytics and tracking API
- **`briefing.go`** - Morning briefing API
- **`diurnal.go`** - Diurnal baseline API
- **`baseline.go`** - Baseline management API
- **`feedback.go`** - User feedback API
- **`security.go`** - Security mode API
- **`alerts.go`** - Alert management
- **`guided.go`** - Guided troubleshooting API

## Key Configuration Files

### Project Root Configuration

1. **`/home/coding/spaxel/Dockerfile`**
   - Multi-stage Docker build configuration
   - Builds Go binary with embedded dashboard
   - Fetches ESP32-S3 firmware from GitHub releases
   - Creates minimal runtime image (distroless)

2. **`/home/coding/spaxel/docker-compose.yml`**
   - Production deployment configuration
   - Service definition with host networking (required for mDNS)
   - Volume mounts for data persistence
   - Environment variable configuration
   - Resource limits and health check

3. **`/home/coding/spaxel/VERSION`**
   - Single source of truth for release version
   - Used by both firmware and Go binary

4. **`/home/coding/spaxel/go.work`**
   - Go workspace configuration
   - References mothership, cmd/sim, and test/acceptance modules

### Dashboard-Specific Configuration

5. **`/home/coding/spaxel/dashboard/package.json`**
   - npm package configuration
   - Test scripts (Jest for unit tests, Playwright for a11y)
   - Development dependencies (TypeScript, testing frameworks)

6. **`/home/coding/spaxel/dashboard/manifest.json`**
   - Progressive Web App (PWA) manifest
   - App shortcuts for Live View and Fleet Status
   - Icon definitions for various sizes
   - Theme colors and display mode

7. **`/home/coding/spaxel/dashboard/sw.js`**
   - Service Worker for offline functionality
   - Caches static assets for performance
   - Never caches live WebSocket or REST API data

8. **`/home/coding/spaxel/dashboard/tsconfig.json`**
   - TypeScript compilation configuration

9. **`/home/coding/spaxel/dashboard/jest.config.js`**
   - Jest unit test configuration

10. **`/home/coding/spaxel/dashboard/playwright.config.js`**
    - Playwright end-to-end test configuration

### Build and Deployment

11. **Build Process:**
    - Dashboard files are copied into `/cmd/mothership/dashboard/` during build
    - `go:embed` directive embeds them in the Go binary (production)
    - Development builds serve from filesystem
    - Production builds serve from embedded filesystem

12. **HTTP Serving:**
    - Static files served via `dashboardStaticHandler()` in `main.go`
    - WebSocket endpoint: `/ws/dashboard` for real-time updates
    - PWA manifest served at `/manifest.json`
    - Service worker at `/sw.js`

## Data Flow Architecture

```
Browser → HTTP/WebSocket → Go Backend (mothership)
                            ↓
                      Dashboard Hub
                            ↓
                      WebSocket broadcast
                            ↓
                      Real-time updates (10 Hz)

Browser ← REST API → Go API Handlers
                         ↓
                   SQLite Database
```

## Entry Points

### HTTP Routes
- `GET /` → Home page (index.html)
- `GET /live` → 3D live view (live.html)
- `GET /simple` → Simple mode (simple.html)
- `GET /ambient` → Ambient mode (ambient.html)
- `GET /fleet` → Fleet status (fleet.html)
- `GET /setup` → Setup wizard (setup.html)
- `GET /integrations` → Integrations (integrations.html)
- `GET /simulator` → Simulator (simulator.html)
- `GET /ws/dashboard` → WebSocket connection

### WebSocket Message Types
From server to client:
- `snapshot` - Initial complete state
- Incremental updates: blobs, nodes, zones, links, triggers, confidence, predictions, events

From client to server:
- `replay_seek` - Seek to timestamp
- `replay_play` - Start replay playback
- `replay_pause` - Pause replay
- `replay_set_params` - Adjust pipeline parameters
- `request_explain` - Request blob explanation

## Technology Stack

### Frontend
- **3D Rendering:** Three.js (hardware-accelerated WebGL)
- **Styling:** Plain CSS with design tokens
- **No Build Tools:** Vanilla JavaScript, no framework
- **Progressive Web App:** Service worker, manifest, installable

### Backend
- **Language:** Go 1.25
- **WebSocket:** gorilla/websocket
- **Routing:** go-chi/chi v5
- **Database:** SQLite (modernc.org/sqlite - pure Go, no CGO)
- **Embed:** go:embed for dashboard files

## Build Commands

```bash
# Development build (filesystem serving)
cd mothership && go build -o spaxel ./cmd/mothership

# Production build (embedded dashboard)
cd mothership && go build -tags=embed -o spaxel ./cmd/mothership

# Run tests
cd dashboard && npm test
cd dashboard && npm run test:a11y

# Type checking
cd dashboard && npm run typecheck

# Full Docker build
docker build -t spaxel:latest .
```

## Important Notes

1. **Dashboard is embedded in production:** The dashboard directory is embedded into the Go binary using `go:embed` in production builds, meaning changes require rebuilding the container image.

2. **No frontend build process:** The dashboard uses vanilla JavaScript with no bundling or transpilation, making it simple to deploy and debug.

3. **Real-time updates via WebSocket:** The dashboard receives updates at 10 Hz through the `/ws/dashboard` WebSocket connection.

4. **PWA support:** The dashboard is installable as a Progressive Web App with offline support through the service worker.

5. **Accessibility:** Extensive Playwright testing ensures the dashboard meets accessibility standards.

6. **Multi-mode interface:** Supports expert mode (full 3D), simple mode (card-based), and ambient mode (wall-mount display).
