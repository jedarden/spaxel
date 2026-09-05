## Discovery Results

**Mothership Dashboard Location:** `/home/coding/spaxel/dashboard/`

**Git Repository Status:** The dashboard is NOT a separate git repository. It is part of the main spaxel monorepo at `https://git.ardenone.com/jedarden/spaxel.git`.

**Dashboard Structure:**
- **Technology:** Vanilla JavaScript + Three.js (no build framework)
- **Entry Points:**
  - `index.html` - Main dashboard
  - `live.html` - Live 3D view with Three.js
  - `fleet.html` - Fleet status table
  - `setup.html` - Setup/calibration interface
  - `simple.html` - Simple mode (card-based UI)
  - `ambient.html` - Ambient display mode for wall-mounted tablets
  - `integrations.html` - Integration settings
  - `simulator.html` - CSI simulator interface

**Key Directories:**
- `dashboard/js/` - JavaScript modules
- `dashboard/css/` - Stylesheets
- `dashboard/static/` - Static assets (icons, additional JS/CSS)
- `dashboard/tests/` - Accessibility tests (Playwright + axe-core)
- `dashboard/types/` - TypeScript type definitions

**Integration with Mothership:**
- Dashboard is embedded into the Go mothership binary via `//go:embed` directive
- Served as static files by the HTTP server at port 8080
- WebSocket endpoint: `/ws/dashboard` for real-time updates at 10 Hz

**Testing:**
- Unit tests with Jest
- Accessibility gate using `@axe-core/playwright` - CI runs this before container build
- Command: `npm run test:a11y`

**Related Components:**
- `mothership/` - Go backend (cmd/mothership/, internal/ packages)
- `firmware/` - ESP32-S3 firmware (ESP-IDF C project)

The dashboard is a self-contained web UI that gets embedded into the mothership Go binary at build time.
