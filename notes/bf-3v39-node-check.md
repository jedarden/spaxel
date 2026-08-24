# Node Verification for bf-3v39 (presence-detection)

**Date:** 2026-08-24
**Mothership:** https://spaxel.ardenone.com
**Task:** Verify first node is CONNECTED and streaming CSI
**Verified:** 2026-08-24 12:00 UTC

## Findings

### Mothership Status (via public `/healthz` endpoint)
```json
{
  "status": "ok",
  "uptime_s": 781539,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

**Result:** Mothership is healthy and running (version 0.2.24, uptime ~9 days), but reports **0 nodes connected**.

### API Access Limitations

The `/api/nodes` endpoint that would provide detailed node status (connection state, CSI frame rate, last seen timestamps) is **protected by Google OAuth** via Traefik reverse proxy. As an agent, I cannot authenticate through the OAuth flow.

#### Public Endpoints (No Auth Required)
- `/healthz` - System health check (✓ Accessed successfully)
- `/api/auth/status` - PIN configuration status
- `/api/provision` - Node provisioning
- `/ws/node` - WebSocket for node ingestion

#### Protected Endpoints (Require OAuth)
- `/api/nodes` - Node list with connection status
- `/api/status` - Detailed system status
- `/api/links` - CSI link status and frame rates

## Required Verification (Operator Action Needed)

To confirm actual node status and CSI streaming, please run these authenticated curls:

```bash
# Get list of all nodes with connection status
curl -s https://spaxel.ardenone.com/api/nodes | jq .

# Get detailed status for first node (replace <MAC>)
curl -s https://spaxel.ardenone.com/api/nodes/<MAC> | jq .

# Get link status with CSI frame rates
curl -s https://spaxel.ardenone.com/api/links | jq .
```

### What to Look For

1. **Node Connection Status:**
   - `status: "CONNECTED"` (not "DISCONNECTED" or "UNPAIRED")
   - `last_seen_ms` should be recent (< 5 seconds ago for actively streaming node)

2. **CSI Frame Rate:**
   - Check `/api/links` endpoint for each link
   - `packet_rate` should be ~20 Hz (configurable rate)
   - Recent CSI timestamps indicate active streaming

3. **Link Health:**
   - Links between nodes should show non-zero `packet_rate`
   - `health_score` should be > 50 for healthy ambient traffic

## If Node Shows Disconnected

### Troubleshooting Runbook

#### 1. Verify Node Power and Network
```bash
# Check node logs via serial/USB if accessible
# Look for WiFi connection messages and mothership connection attempts
```

#### 2. Check NVS Configuration (passive_bss Key)
The presence detection feature requires the home AP BSSID stored in NVS:

```c
// On the ESP32, check NVS storage:
// Key: "passive_bss"
// Value: Should be the BSSID of the home WiFi AP (e.g., "aa:bb:cc:dd:ee:ff")
```

**To verify/set the BSSID:**
1. Access node serial console or SSH
2. Use ESP-IDF tool: `python -m esptool --port <PORT> read_nvsm <BSSID>`
3. Or via web provisioning interface if available

#### 3. Verify WiFi Credentials
- Ensure `SPAXEL_WIFI_SSID` and `SPAXEL_WIFI_PASSWORD` are set in mothership environment
- Network settings are now centralized in the mothership (ADR-005)
- Nodes fetch credentials from mothership during provisioning

#### 4. Check Mothership Provisioning
```bash
# Verify provisioning endpoint is accessible
curl -s https://spaxel.ardenone.com/api/provision
```

#### 5. Check Firewall/Port Access
- Node must reach mothership on port 443 (HTTPS) or 80 (HTTP)
- WebSocket endpoint `/ws/node` must be accessible

#### 6. Review Node Logs
- Check for WiFi connection failures
- Look for TLS handshake errors
- Verify mothership hostname resolution

## Next Steps

1. **Operator:** Run the authenticated curl commands above and share output
2. **If connected:** Document CSI frame rate and confirm streaming is active
3. **If disconnected:** Follow troubleshooting runbook starting with step 1
4. **Update this file** with actual node status and CSI rate evidence

## Parent Bead Reference

This verification is split-child 1 of **bf-3v39** (presence-detection verification).
Prerequisite **bf-2po1** closed 2026-08-07.
