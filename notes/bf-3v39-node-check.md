# Node Verification: spaxel-9c1a4858

**Task:** Verify first node is CONNECTED and streaming CSI on live mothership at https://spaxel.ardenone.com

**Date:** 2026-08-21

## Background

Split-child of bf-3v39 (presence-detection verification). Prerequisite bf-2po1 closed 2026-08-07.

The live mothership at https://spaxel.ardenone.com is OAuth-gated (Google SSO via auth.ardenone.com). Direct agent access is blocked by the forward-auth middleware.

## Target Verification

We need to confirm:
1. **Node status:** First ESP32 node shows CONNECTED (not OFFLINE or STALE)
2. **CSI streaming:** Frames are arriving with recent timestamps at expected rate (~20 Hz for active mode, ~10 Hz beacon rate for passive mode)
3. **Passive radar:** If in passive mode, confirm the node is filtering for the correct AP BSSID

## API Access Paths

From `mothership/internal/auth/handler.go`:
- **Public paths** (no auth required):
  - `/healthz` - System health check
  - `/api/auth/status` - PIN configuration status
  - `/api/provision` - Provisioning endpoint
  - `/ws/node` - Node WebSocket (uses HMAC token, not OAuth)
  
- **Authenticated paths** (requires Google SSO session):
  - `/api/nodes` - Node registry and status
  - `/api/status` - System status
  - `/api/links` - Active CSI links (if any)

## Health Check Results (✓ OBTAINED)

**Executed:** `curl -s https://spaxel.ardenone.com/healthz`

**Response:**
```json
{
  "status": "ok",
  "uptime_s": 515546,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

**Findings:**
- ✅ Mothership is healthy and running (uptime: ~5.97 days)
- ✅ Database is OK
- ✅ No load shedding (level 0)
- ✅ Version 0.2.24 deployed
- ❌ **ZERO nodes connected** (`nodes_online: 0`)
- ❌ **No CSI streaming possible** (no nodes = no CSI)

## Conclusion: NODE NOT CONNECTED

The first ESP32 node is **NOT CONNECTED** to the live mothership. This is a documented, closable outcome per the task acceptance criteria.

**What's missing:**
1. No node has registered with the mothership (nodes_online: 0)
2. Cannot verify CSI streaming because there are no nodes
3. Cannot verify passive BSSID configuration because there are no nodes

**Next:** Operator needs to troubleshoot why the ESP32 node is not connecting to `https://spaxel.ardenone.com`. See troubleshooting runbook below.

## Required: Operator-Run curl Commands

Please run these commands and paste the output:

### 1. System Health Check (public endpoint)
```bash
curl -s https://spaxel.ardenone.com/healthz
```

### 2. Node Status (requires authenticated session)
```bash
# After opening https://spaxel.ardenone.com in browser and authenticating with Google SSO:
curl -s https://spaxel.ardenone.com/api/nodes \
  -H "Cookie: spaxel_session=<YOUR_SESSION_COOKIE>"
```

### 3. System Status (requires authenticated session)
```bash
curl -s https://spaxel.ardenone.com/api/status \
  -H "Cookie: spaxel_session=<YOUR_SESSION_COOKIE>"
```

### 4. Active Links (CSI streaming verification)
```bash
curl -s https://spaxel.ardenone.com/api/links \
  -H "Cookie: spaxel_session=<YOUR_SESSION_COOKIE>"
```

## How to Get Session Cookie

1. Open https://spaxel.ardenone.com in browser
2. Authenticate with Google SSO
3. Open browser DevTools (F12) → Application/Storage → Cookies
4. Find `spaxel_session` cookie value
5. Copy the value (64-character hex string)

## Expected Evidence

### Connected Node (Expected output from /api/nodes):
```json
[
  {
    "mac": "AA:BB:CC:DD:EE:FF",
    "name": "...",
    "role": "rx" or "tx_rx" or "passive",
    "status": "online",           // ← Critical: not "offline" or "stale"
    "last_seen_ms": 1692634567890, // ← Recent timestamp (within last 60 seconds)
    "wifi_rssi_dbm": -45,
    "uptime_ms": 3600000,
    "csi_rate_hz": 20 or 10       // ← Shows configured rate
  }
]
```

### CSI Streaming (Expected output from /api/links):
```json
[
  {
    "tx_mac": "...",
    "rx_mac": "...",
    "last_frame_ms": 1692634567890,  // ← Recent timestamp (within last second)
    "frame_count": 12345,
    "delta_rms": 0.08,                    // ← Shows recent motion detection
    "snr": 25.3,
    "pdr": 0.98
  }
]
```

## Passive Radar Specific Checks

If the node is in passive mode (using router as TX):
1. Check `/api/nodes` for `role: "passive"`
2. Verify node has `passive_bssid` set in database
3. CSI rate should be ~10 Hz (typical beacon interval)
4. Last frame timestamps should be within last 2 seconds

## Troubleshooting Runbook (if node NOT streaming)

### Issue 1: Node shows OFFLINE
**Symptoms:** `status: "offline"`, `last_seen_ms` is old (>60 seconds ago)

**Checks:**
- Is the ESP32 powered? Check USB/Power connection
- Does the node have WiFi connectivity? Check captive portal AP `spaxel-XXXX`
- Can mothership reach the node? Check `SPAXEL_MDNS_ENABLED` and network routing

**Recovery:**
1. Check node's serial console or Web Serial for error messages
2. Verify NVS `wifi_ssid` and `wifi_pass` are correct
3. Re-provision via captive portal or Web Serial

### Issue 2: Node shows STALE
**Symptoms:** `status: "stale"`, connected but not sending health/CSI

**Checks:**
- Is WebSocket connection alive? Check for disconnect logs
- Is firmware crashed? Look for panic messages or restart loop
- Is mothership reachable from node? Check network path

**Recovery:**
1. Restart node via power cycle
2. Check mothership logs for ingestion errors
3. Verify node token validation (should be auto-generated)

### Issue 3: Node ONLINE but NO CSI frames
**Symptoms:** Node status shows online, but `/api/links` is empty or has stale frames

**Checks:**
- **CRITICAL:** Is CSI armed? (Known bug bf-5x46 — CSI may not arm on boot if role matches persisted role)
- Is node in correct role? (`tx`, `rx`, `tx_rx`, or `passive`)
- For passive mode: Is `passive_bssid` NVS key set to router's BSSID?
- Are packets being captured? Check firmware CSI callback logs

**Recovery:**
1. Force role change to re-arm CSI: `PATCH /api/nodes/{mac}` with new role
2. For passive mode, verify NVS `passive_bss` contains correct router BSSID
3. Reboot node to re-initialize CSI hardware

### Issue 4: Passive BSSID not set
**Symptoms:** Node in passive role but no frames arriving

**Root Cause:** According to plan, NVS key `passive_bss` should be auto-detected during provisioning but may be empty.

**Check:**
```bash
# From mothership database (if accessible):
SELECT passive_bssid FROM nodes WHERE mac = 'AA:BB:CC:DD:EE:FF';
```

**Recovery:**
1. Manual AP BSSID entry via captive portal
2. Re-provision node with correct BSSID
3. Verify AP is actually transmitting beacons on expected channel

## Next Steps

WAITING for operator to provide:
1. `/healthz` output (public)
2. `/api/nodes` output (authenticated)
3. `/api/status` output (authenticated)
4. `/api/links` output (authenticated)

Once data received, I will:
- Confirm node is CONNECTED and streaming
- Document CSI rate and timestamp freshness
- Identify any gaps and create troubleshooting runbook if needed
- Comment on bead spaxel-9c1a4858 with findings
