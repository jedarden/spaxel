# WiFi Credential Provisioning Flow Audit

**Date:** 2026-08-11  
**Purpose:** Document the complete WiFi credential handling in the provisioning flow

## Overview

This document describes how WiFi credentials flow from the dashboard → mothership → ESP32 device, including validation requirements, error messages, and the database precedence model (per ADR-005).

**Last Updated:** 2026-08-11

**Implementation Status:** ✅ **FULLY IMPLEMENTED**

The `SPAXEL_WIFI_SSID` and `SPAXEL_WIFI_PASSWORD` environment variables are **fully implemented** per ADR-005:
- `mothership/internal/config/config.go` reads both env vars at startup (lines 240-244)
- `mothership/cmd/mothership/main.go` seeds the database on first boot via `seedWiFiCredentialsIfFirstBoot()` (lines 655-695, called at line 825)
- Database becomes source of truth after first boot; env vars are ignored on subsequent boots

---

## Credential Flow Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. DASHBOARD (Settings)                                                              │
│  User enters fleet WiFi credentials once in Settings > Network panel                │
│  → PUT /api/settings/network { wifi_ssid, wifi_password }                      │
│  → Stored in SQLite settings table as "network_wifi_ssid" and "network_wifi_password"│
│  → Password is write-only (never returned in responses)                               │
└─────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 2. DASHBOARD (Onboarding Wizard)                                                      │
│  Two paths to WiFi credentials:                                                      │
│                                                                                      │
│  Path A (Default):                                                                  │
│  → Wizard checks /api/settings/network at start (fetchFleetNetworkSettings)         │
│  → If configured, skips WiFi step entirely                                       │
│  → Displays "New nodes will join 'SSID' automatically"                             │
│                                                                                      │
│  Path B (Per-Node Override):                                                        │
│  → User clicks "Advanced: use a different network for this node"                     │
│  → WiFi form appears with SSID/password fields                                     │
│  → User enters credentials for this specific node                                    │
│  → POST /api/provision { wifi_ssid, wifi_pass, ... }                               │
└─────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 3. MOTHERSHIP (Provisioning Server)                                                 │
│  → POST /api/provision endpoint (mothership/internal/provisioning/server.go)    │
│  → Reads fleet network settings from settings provider (in-memory cache of DB)   │
│  → Credential selection logic:                                                    │
│     1. If request body contains wifi_ssid/wifi_pass → use those (override wins)   │
│     2. Else if network_wifi_ssid/network_wifi_password in settings → use those    │
│     3. Else → return 400 error "no wifi_ssid provided and no fleet network configured"│
│  → Generate provisioning payload with selected credentials                           │
│  → Derive node_token = HMAC-SHA256(install_secret, mac)                               │
│  → Return JSON payload to dashboard                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 4. DASHBOARD (Web Serial)                                                           │
│  → Open serial port to ESP32-S3                                                       │
│  → Wait for "SPAXEL READY <MAC>" banner from firmware                                 │
│  → Send {"provision": <payload>} over serial                                      │
│  → Wait for {"ok": true, "mac": "AA:BB:CC:DD:EE:FF"} response                           │
│  → Confirm MAC address matches expected node                                       │
│  → POST /api/provision with confirmed MAC to finalize token                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 5. FIRMWARE (ESP32-S3 NVS)                                                           │
│  → Provisioning window active (first 10-120s after boot)                            │
│  → Receive provisioning JSON over UART                                                │
│  → Validate and parse JSON                                                           │
│  → Write wifi_ssid to NVS (wifi_ssid key, max 32 bytes)                             │
│  → Write wifi_pass to NVS (wifi_pass key, max 64 bytes)                             │
│  → Write node_id, node_token, ms_mdns, ms_port, debug                                │
│  → Write "provisioned" flag LAST (only if all writes succeed)                         │
│  → nvs_commit() → reboot                                                                 │
│  → Connect to WiFi on next boot using stored credentials                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Details

### 1. Dashboard Network Settings Panel

**File:** `dashboard/js/settings-panel.js`

**API Endpoints:**
- `GET /api/settings/network` - Returns `{wifi_ssid, configured: bool}` (password never echoed)
- `PUT /api/settings/network` - Updates SSID and/or password

**Validation Rules:**
```javascript
// SSID validation (network_settings.go:92-101)
- Must not be empty after trim()
- Maximum 32 characters
- Error: "wifi_ssid: must not be empty" or "wifi_ssid: must be 32 characters or fewer"

// Password validation (network_settings.go:109-120)
- Minimum 8 characters (WPA2 requirement) OR empty for open networks
- Error: "wifi_password: must be at least 8 characters (WPA2 minimum) or empty for an open network"
```

**User Experience:**
- Password field shows placeholder "Unchanged (leave blank to keep)" when configured
- Helper text explains: "A password is already saved and will not be shown here. Enter a new one to replace it."
- Status card shows "Configured" or "Not configured"
- Message: "New nodes will join 'SSID' automatically" or "New nodes will prompt for WiFi credentials during onboarding"

---

### 2. Onboarding Wizard

**File:** `dashboard/js/onboard.js`

**States:** `BROWSER_CHECK → CONNECT_DEVICE → PROVISION_WIFI → FLASH_FIRMWARE → DETECT_NODE → CALIBRATE → PLACEMENT → COMPLETE`

**WiFi Step Behavior:**

**Default (Path A):**
```javascript
// Lines 943-954: Fetch fleet network settings
fetchFleetNetworkSettings() → GET /api/settings/network
// Lines 970-996: Auto-skip WiFi step
if (state.fleetNetworkConfigured) {
    // Skip entire WiFi provision step
    // Show: "Fleet network already configured: 'SSID'"
}
```

**Advanced Override (Path B):**
```javascript
// Lines 954-962: User opts into "use a different network for this node"
// Show WiFi SSID/password input fields
// Submit button calls provisionAndSend()
```

**Provisioning Request:**
```javascript
// Lines 1053-1102: POST to /api/provision
POST /api/provision {
    wifi_ssid: <from fleet settings or override>,
    wifi_pass: <from fleet settings or override>,
    mac: optional (for token derivation),
    ms_ip: optional (manual mothership IP),
    debug: optional
}
```

---

### 3. Mothership Provisioning Server

**File:** `mothership/internal/provisioning/server.go`

**Request Schema:**
```go
type provisionRequest struct {
    WifiSSID string `json:"wifi_ssid"`      // Optional override
    WifiPass string `json:"wifi_pass"`      // Optional override
    MAC      string `json:"mac,omitempty"` // For deterministic token
    MsIP     string `json:"ms_ip,omitempty"`
    Debug    bool   `json:"debug,omitempty"`
}
```

**Credential Selection Logic (Lines 218-238):**
```go
// Priority order:
// 1. Explicit request body (override wins)
// 2. Fleet network settings from database
// 3. Error if neither available

wifiSSID := req.WifiSSID
if wifiSSID == "" {
    if v, ok := s.networkSetting(networkSettingWifiSSID); ok {
        wifiSSID = v
    }
}
if wifiPass == "" {
    if v, ok := s.networkSetting(networkSettingWifiPassword); ok {
        wifiPass = v
    }
}
if wifiSSID == "" {
    http.Error(w, "no wifi_ssid provided and no fleet network configured; "+
        "set WiFi credentials in the mothership dashboard under Settings > Network, "+
        "or include wifi_ssid/wifi_pass in the request", http.StatusBadRequest)
    return
}
```

**Response Payload:**
```go
type Payload struct {
    Version   int    `json:"version"`       // Always 1
    WifiSSID  string `json:"wifi_ssid"`
    WifiPass  string `json:"wifi_pass"`
    NodeID    string `json:"node_id"`       // UUID4
    NodeToken string `json:"node_token"`     // HMAC-SHA256(install_secret, mac)
    MsMDNS    string `json:"ms_mdns"`
    MsIP      string `json:"ms_ip,omitempty"`
    MsPort    int    `json:"ms_port"`
    NTPServer string `json:"ntp_server"`
    Debug     bool   `json:"debug"`
}
```

**Error Messages (When Credentials Missing):**

| Scenario | HTTP Status | Error Message |
|----------|-------------|---------------|
| No SSID in request AND no fleet network configured | 400 | "no wifi_ssid provided and no fleet network configured; set WiFi credentials in the mothership dashboard under Settings > Network, or include wifi_ssid/wifi_pass in the request" |
| Invalid JSON body | 400 | "invalid JSON body" |
| Provisioning not ready (no install secret) | 503 | "provisioning not ready (no install secret)" |

---

### 4. Network Settings API

**File:** `mothership/internal/api/network_settings.go`

**Settings Keys (Lines 17-20):**
```go
const (
    networkSettingWifiSSID     = "network_wifi_ssid"
    networkSettingWifiPassword = "network_wifi_password"
)
```

**GET /api/settings/network Response:**
```go
type networkSettingsResponse struct {
    WifiSSID   string `json:"wifi_ssid"`      // SSID or empty string
    Configured bool   `json:"configured"`    // true if both SSID and password set
}
```

**PUT /api/settings/network Request:**
```go
type networkSettingsRequest struct {
    WifiSSID     *string `json:"wifi_ssid,omitempty"`     // Pointer to distinguish empty vs omitted
    WifiPassword *string `json:"wifi_password,omitempty"`
}
```

**Validation Rules:**
```go
// SSID (Lines 92-101):
- Trim whitespace
- Must not be empty after trim
- Maximum 32 characters
- Error: "wifi_ssid: must not be empty"
- Error: "wifi_ssid: must be 32 characters or fewer"

// Password (Lines 109-120):
- If non-empty, must be at least 8 characters
- Empty string allowed for open networks
- Error: "wifi_password: must be at least 8 characters (WPA2 minimum) or empty for an open network"
```

**Security Note:** WiFi password is **write-only**. The GET endpoint never includes it in responses (line 36 comment), matching the MQTT password convention for sensitive credentials.

---

### 5. Settings Storage

**File:** `mothership/internal/api/settings.go`

**Database Schema:**
```sql
CREATE TABLE settings (
    key         TEXT PRIMARY KEY,
    value_json  TEXT NOT NULL,
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);
```

**Storage Keys:**
- `network_wifi_ssid` → fleet-wide WiFi SSID
- `network_wifi_password` → fleet-wide WiFi password

**In-Memory Cache:**
- SettingsHandler maintains an in-memory cache for fast reads
- Updated on every write via `Set()` method
- Cache invalidated on updates
- Loaded from database on startup (lines 89-122)

**Precedence:**
1. **Database settings** (`network_wifi_ssid`/`network_wifi_password`) - authoritative source after first boot
2. **Request body override** - temporary per-node override during provisioning
3. **Environment variables** - first-boot seed only (see below)

---

### 6. Environment Variables (✅ IMPLEMENTED per ADR-005)

**Documentation:** `docs/plan/plan.md` (ADR-005, 2026-08-03)

**Implementation Status:** ✅ **FULLY IMPLEMENTED** (2026-08-11)

**Environment Variables:**
```
SPAXEL_WIFI_SSID     - Optional first-boot seed for fleet WiFi network name
SPAXEL_WIFI_PASSWORD - First-boot seed for passphrase
```

**Implementation Locations:**
- **Config loading:** `mothership/internal/config/config.go` lines 240-244
- **Seeding logic:** `mothership/cmd/mothership/main.go` lines 655-695 (`seedWiFiCredentialsIfFirstBoot`)
- **Startup call:** Line 825 in main (called after settings handler initialization)

**Behavior:**
- **First boot only:** If BOTH env vars are non-empty AND database has no `network_wifi_ssid` setting, the database is seeded with these values
- **Both required:** If only one env var is set, seeding is skipped with a log message
- **Ignored after first boot:** Once database has network settings, env vars are ignored (DB is source of truth per ADR-005)
- **Logging:** Successful seeding logs "[CONFIG] Seeded network settings from SPAXEL_WIFI_* environment variables (first boot - will not run again)"

**Actual Behavior:**
- ✅ WiFi credentials CAN be seeded via env vars on first boot
- ✅ Dashboard Settings > Network panel is the primary interface for setting/changing fleet WiFi
- ✅ After first boot, changing env vars has no effect (DB is authoritative)

---

### 7. Firmware Provisioning (ESP32-S3)

**File:** `firmware/main/provision.c`

**NVS Keys:**
```c
#define NVS_KEY_WIFI_SSID "wifi_ssid"  // Max 32 bytes
#define NVS_KEY_WIFI_PASS "wifi_pass"  // Max 64 bytes
```

**Validation (Lines 147-158):**
```c
// WiFi SSID - REQUIRED
esp_err_t err = nvs_set_str("wifi_ssid", wifi_ssid, strlen(wifi_ssid));
if (err != ESP_OK || strlen(wifi_ssid) == 0) {
    // Return error: "WiFi SSID is required"
}

// WiFi Password - OPTIONAL
if (strlen(wifi_pass) > 0) {
    err = nvs_set_str("wifi_pass", wifi_pass, strlen(wifi_pass));
    // Non-zero length passwords stored; empty string allowed for open networks
}
```

**NVS Write Sequence:**
1. Erase "spaxel" namespace
2. Write wifi_ssid (required, non-empty)
3. Write wifi_pass (optional, any length including empty)
4. Write node_id, node_token, ms_mdns, ms_port, debug
5. Write "provisioned" = 1 **LAST** (only if all above succeed)
6. nvs_commit()
7. Reboot

**Provisioning Window:**
- Fresh boards: 120 seconds
- Re-provisioning: 15 seconds
- "SPAXEL READY <MAC>" broadcast signal at 1 Hz
- Accepts `{"provision": <payload>}` JSON

---

## Error Messages Summary

### User-Facing Error Messages

| Context | Error Message | HTTP Status | Component |
|---------|--------------|-------------|-----------|
| Onboarding, no credentials | "No WiFi credentials configured. Set WiFi credentials in the mothership dashboard under Settings > Network, or include wifi_ssid/wifi_pass in the request." | Displayed in wizard UI (derived from 400 response) | Dashboard → API |
| API validation, empty SSID | "wifi_ssid: must not be empty" | 400 | Network Settings API |
| API validation, SSID too long | "wifi_ssid: must be 32 characters or fewer" | 400 | Network Settings API |
| API validation, short password | "wifi_password: must be at least 8 characters (WPA2 minimum) or empty for an open network" | 400 | Network Settings API |
| Provisioning, no credentials | "no wifi_ssid provided and no fleet network configured; set WiFi credentials in the mothership dashboard under Settings > Network, or include wifi_ssid/wifi_pass in the request" | 400 | Provisioning API |
| Provisioning, not ready | "provisioning not ready (no install secret)" | 503 | Provisioning API |
| Provisioning, invalid JSON | "invalid JSON body" | 400 | Provisioning API |
| Firmware, missing SSID | "WiFi SSID is required" (internal firmware error) | — | Firmware |

### Log Messages

| Level | Message | Component |
|-------|--------|-----------|
| INFO | "provisioning: generated payload node_id=%s mac=%s" | Provisioning server |
| WARN | "provisioning: invalid SPAXEL_INSTALL_SECRET, will use persisted secret" | Provisioning server |
| INFO | "provisioning: loaded install secret from %s" | Provisioning server |
| INFO | "provisioning: generated new install secret at %s" | Provisioning server |
| ERROR | "Failed to save network settings: %v" | Network Settings API |
| WARN | "OTA disabled: %v" | Config (advertised URL derivation) |

---

## Database vs Environment Variable Precedence

### Current Implementation (✅ Env Var Support Implemented)

**Precedence:**
1. **Database settings** (`network_wifi_ssid`/`network_wifi_password`) - authoritative source after first boot
2. **Request body override** - Per-node override during provisioning
3. **Environment variables** - ✅ **IMPLEMENTED** - first-boot seed only

**How It Works:**
1. **First boot with env vars:** If `SPAXEL_WIFI_SSID` AND `SPAXEL_WIFI_PASSWORD` are both set AND database has no network settings, seed the database with env var values
2. **Subsequent boots:** Database is authoritative; env vars are ignored even if set
3. Dashboard Settings > Network stores/retrieves credentials from SQLite `settings` table
4. Onboarding wizard fetches from `/api/settings/network` and skips WiFi input if already configured
5. Provisioning server reads from in-memory cache of settings table
6. Request body can override for specific node

### Env Var Status

| Environment Variable | Documented | Implemented | Effect |
|---------------------|-------------|------------|--------|
| `SPAXEL_WIFI_SSID` | ✅ (ADR-005) | ✅ | Seeds DB on first boot only |
| `SPAXEL_WIFI_PASSWORD` | ✅ (ADR-005) | ✅ | Seeds DB on first boot only |
| `SPAXEL_INSTALL_SECRET` | ✅ | ✅ | Seeds or loads install secret |

**Implementation Details:**
- **Config loading:** `mothership/internal/config/config.go` lines 240-244 reads both env vars
- **Seeding logic:** `mothership/cmd/mothership/main.go::seedWiFiCredentialsIfFirstBoot()` (lines 655-695)
- **Startup call:** Line 825 in main (called after settings handler initialization)
- **Database keys:** Stores as `network_wifi_ssid` and `network_wifi_password` in settings table

**Implications:**
- ✅ **Scripted/headless deployments CAN pre-configure WiFi** via env vars on first boot
- ✅ **Dashboard UI is still primary interface** for changing WiFi credentials after first boot
- ✅ **Changing env vars after first boot has no effect** (DB is authoritative)
- ✅ **All provisioning paths work:** env var seed, dashboard entry, per-node override

---

## Validation Requirements Summary

### API Layer (network_settings.go)

**SSID:**
- Required: Yes (must not be empty after trim)
- Max length: 32 characters
- Validation: `trimmed != ""` AND `len(trimmed) <= 32`

**Password:**
- Required: No (can be empty for open networks)
- Min length: 8 characters (if non-empty)
- Validation: `pass == ""` OR `len(pass) >= 8`
- WPA2 minimum: 8 characters

### Provisioning API Layer (server.go)

**SSID:**
- Required: Yes (must have a value from either request body or settings)
- Validation: Must be non-empty string
- Error: 400 "no wifi_ssid provided and no fleet network configured..."

**Password:**
- Required: No (optional, allows open networks)
- Validation: None (any string accepted, including empty)
- Note: Firmware accepts any password length; API enforces 8-char minimum

### Firmware Layer (provision.c)

**SSID:**
- Required: Yes (must be non-empty string)
- Storage: NVS `wifi_ssid` key, max 32 bytes
- Validation: Non-zero length check
- Error: Firmware rejects provisioning with error response

**Password:**
- Required: No (optional)
- Storage: NVS `wifi_pass` key, max 64 bytes
- Validation: None (any length including empty)
- Empty string = open network

---

## Code Paths Requiring WiFi Credentials

### Primary Paths

1. **Normal provisioning (fleet network configured):**
   - Dashboard Settings > Network → `PUT /api/settings/network`
   - Onboarding wizard → `GET /api/settings/network` → auto-skip WiFi step
   - `POST /api/provision` → reads from settings provider
   - Payload includes credentials from database

2. **Per-node override (advanced mode):**
   - Onboarding wizard → "Advanced: use a different network for this node"
   - User enters SSID/password in wizard form
   - `POST /api/provision {wifi_ssid, wifi_pass, ...}`
   - Request body overrides database settings
   - Payload includes override credentials

3. **Captive portal recovery:**
   - Device enters AP mode (`spaxel-XXXX`)
   - User connects to `192.168.4.1`
   - Captive portal shows credentials input form
   - Form submits to same provisioning endpoint
   - Normal provisioning flow from there

### Degraded Paths

**No WiFi credentials configured:**
- Onboarding wizard shows error banner
- Provisioning API returns 400 error
- Firmware cannot be provisioned
- Device remains in captive portal loop
- User must either:
  - Configure fleet network in dashboard Settings > Network, OR
  - Use "Advanced" mode to enter credentials per-node

---

## Recommendations

### For Deployment

1. **First-Time Setup:**
   - Start mothership container
   - Open dashboard in browser
   - Complete first-run PIN setup
   - Navigate to Settings > Network
   - Enter fleet WiFi SSID and password
   - All subsequent nodes will auto-join this network

2. **Headless/Scripted Deployment:**
   - **Current limitation:** Env vars `SPAXEL_WIFI_*` do not work
   - **Workaround:** Call `PUT /api/settings/network` via API after startup:
     ```bash
     curl -X PUT http://mothership:8080/api/settings/network \
       -H "Content-Type: application/json" \
       -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
     ```

3. **Per-Network Nodes:**
   - Use onboarding wizard's "Advanced" mode
   - Or include credentials in `POST /api/provision` body

### For Implementation

1. **Implement ADR-005 env var seeding** (if desired):
   - Add startup code in `main.go` to read `SPAXEL_WIFI_SSID` and `SPAXEL_WIFI_PASSWORD`
   - Check if `network_wifi_ssid` exists in database
   - If not, seed from env vars **once only**
   - Requires migration or manual DB initialization

2. **Add validation to firmware** (optional improvement):
   - Enforce minimum password length on device
   - Or keep accepting any length (current behavior)

3. **Add validation to provisioning API** (current gap):
   - Currently accepts any password length
   - Consider enforcing WPA2 minimum at API layer too
   - Firmware already enforces minimum 8 chars; inconsistent with API

---

## Appendix: File Reference

### Dashboard

- `dashboard/js/onboard.js` - Onboarding wizard WiFi step
- `dashboard/js/settings-panel.js` - Settings > Network panel
- `dashboard/js/fleet.js` - Node list with unpaired detection

### Mothership API

- `mothership/internal/api/network_settings.go` - GET/PUT /api/settings/network
- `mothership/internal/api/settings.go` - Settings storage and cache
- `mothership/internal/provisioning/server.go` - POST /api/provision

### Mothership Main

- `mothership/cmd/mothership/main.go` - Server wiring (no WiFi env var code)

### Firmware

- `firmware/main/provision.c` - Serial provisioning and NVS writes
- `firmware/main/spaxel.h` - NVS key constants

### Documentation

- `docs/plan/plan.md` - Architecture Decision Record ADR-005
