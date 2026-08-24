# Node Verification Results - bf-3v39 (2026-08-24)

## Executive Summary

**STATUS: ❌ NO NODES CONNECTED**

The live mothership at https://spaxel.ardenone.com is currently reporting **ZERO connected nodes**. This means the first ESP32 node is NOT CONNECTED and is NOT streaming CSI frames.

## Evidence

### Public Endpoint Check
```bash
curl -s https://spaxel.ardenone.com/healthz
```

**Response:**
```json
{
  "status":"ok",
  "uptime_s":784615,
  "version":"0.2.24",
  "nodes_online":0,
  "db":"ok",
  "shedding_level":0,
  "reason":"no nodes connected"
}
```

### Key Findings

| Metric | Value | Interpretation |
|--------|-------|----------------|
| Mothership Status | `ok` | Service is healthy and running |
| Uptime | 784,615 seconds (~9 days) | Stable deployment |
| Version | 0.2.24 | Current production build |
| **Nodes Online** | **0** | **CRITICAL: No ESP32 nodes connected** |
| Database | `ok` | Backend storage healthy |
| Shedding Level | 0 | No load shedding |
| Reason | "no nodes connected" | Explicit confirmation |

## Authentication Limitations

The `/api/nodes` and `/api/status` endpoints require OAuth authentication (Google SSO via auth.ardenone.com), which cannot be automated from an agent session:

```bash
curl -s https://spaxel.ardenone.com/api/nodes
# Returns: HTML redirect to OAuth flow
```

However, the public `/healthz` endpoint provides sufficient evidence of node connectivity status.

## CSI Streaming Status

**RESULT: NOT STREAMING**

Since there are 0 nodes connected, CSI frames cannot be arriving. The presence detection system is currently non-functional due to lack of connected sensor nodes.

## Required Actions

This is a **critical infrastructure issue** that requires immediate operator intervention. The first ESP32 node should have been operational per the parent bead (bf-3v39).

### Next Steps for Operator

1. **Check ESP32 Power**: Verify the first ESP32 node has power and is booted
2. **Verify WiFi Connection**: Ensure the device can reach the home AP
3. **Check NVS Configuration**: Verify `passive_bss` holds the correct home AP BSSID
4. **Review Mothership Logs**: Check for connection attempts or errors in mothership logs
5. **Inspect Device Serial**: Connect via serial to see boot/connection logs

## Troubleshooting Runbook

See the detailed runbook below for step-by-step diagnostics.

---

## Mothership Authentication System Notes

### Public Endpoints (No Auth Required)
- `/healthz` - Health check and status
- `/api/auth/status` - PIN configuration check
- `/api/auth/setup` - First-time PIN setup
- `/api/auth/login` - PIN-based login
- `/api/provision` - Device token provisioning
- `/ws/node` - Node WebSocket connections
- `/firmware/*` - Firmware files (integrity protected)

### Authenticated Endpoints
- `/api/nodes` - Node list and status (requires OAuth or demo mode)
- `/api/status` - Detailed system status (requires OAuth or demo mode)
- All POST/PUT/PATCH/DELETE operations (require OAuth)

### Device Token System
- Node tokens are HMAC-SHA256(tokens): `HMAC-SHA256(install_secret, mac_address)`
- Tokens allow nodes to authenticate without interactive OAuth
- Install secret stored in database or `SPAXEL_INSTALL_SECRET` env var

### Demo Mode
- If enabled, allows GET requests without authentication
- Blocks all POST/PUT/PATCH/DELETE operations regardless
- Controlled via configuration setting
