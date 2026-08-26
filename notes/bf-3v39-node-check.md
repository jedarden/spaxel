# Node Connection & CSI Streaming Verification

**Date:** Initial check 2026-08-24; latest live check 2026-08-26 UTC
**Bead:** spaxel-9c1a4858 (split-child 1 of bf-3v39)  
**Mothership:** https://spaxel.ardenone.com  
**Purpose:** Verify first ESP32 node shows CONNECTED and CSI frames are arriving

## Latest Live Observation (2026-08-26T03:00:39Z)

The public health probe returned HTTP 200 with:

```json
{"status":"ok","uptime_s":927354,"version":"0.2.24","nodes_online":0,"db":"ok","shedding_level":0,"reason":"no nodes connected"}
```

This is a current negative result: the mothership has zero connected node WebSocket sessions, so no CSI frame can be arriving and no CSI frame rate exists to measure. The health probe was also observed at 02:59:10Z with `nodes_online: 0`.

The following read-only endpoints remain behind the external Google OAuth forward-auth gate and returned HTTP 307 redirects to `accounts.google.com` from this unauthenticated agent:

| Endpoint | Result |
|---|---|
| `GET /api/nodes` | 307 OAuth redirect |
| `GET /api/status` | 307 OAuth redirect |
| `GET /api/links` | 307 OAuth redirect |

Source inspection found no agent/service read-token route. `mothership/internal/auth/handler.go` registers session/PIN routes only; node HMAC tokens are used by `/ws/node` and OTA validation, not by the dashboard REST reads. `/api/provision` is a public **POST** that mints node credentials, so it was not called during this read-only verification.

## Current Status

### Mothership Health (from `/healthz` endpoint, latest check)
```json
{
  "status": "ok",
  "uptime_s": 927354,           // ~10.7 days uptime
  "version": "0.2.24",
  "nodes_online": 0,             // ❌ NO NODES CONNECTED
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

### API Access Status
- ✅ `/healthz` - Publicly accessible, returns 200
- ❌ `/api/status` - OAuth-gated (Google SSO via auth.ardenone.com)
- ❌ `/api/nodes` - OAuth-gated (Google SSO via auth.ardenone.com)
- ❌ `/api/links` - OAuth-gated (alternative CSI/link view)

### Finding Summary
**❌ NO NODES ARE CURRENTLY CONNECTED TO THE MOTHERSHIP.**

The mothership is healthy and has been running continuously for ~10.7 days, but zero nodes are online. This means:
- No CSI frames are being streamed
- No presence detection is occurring
- The system is in an idle state with no fleet

## Next Steps (Operator Action Required)

The API endpoints that would show registered nodes (even if offline) are behind OAuth authentication. To proceed with verification, please run the following authenticated curl commands and paste the output:

### 1. Check Registered Nodes
```bash
# After authenticating to https://spaxel.ardenone.com
curl -s "https://spaxel.ardenone.com/api/nodes" -H "Cookie: spaxel_session=<your-session-cookie>"
```

Expected output should show:
- List of all registered nodes (by MAC address)
- Node status: ONLINE / STALE / OFFLINE
- Last seen timestamp for each node
- Firmware version per node

### 2. Check System Status
```bash
curl -s "https://spaxel.ardenone.com/api/status" -H "Cookie: spaxel_session=<your-session-cookie>"
```

Expected output should show:
- Total node count
- Active blob count
- Detection quality score
- Current CSI rate (if any nodes streaming)

### 3. Check CSI Links (Alternative)
```bash
curl -s "https://spaxel.ardenone.com/api/links" -H "Cookie: spaxel_session=<your-session-cookie>"
```

This would show:
- All active links between nodes
- Per-link CSI frame rates
- Link quality metrics

## Troubleshooting Runbook (If Node Exists But Not Connected)

If `/api/nodes` shows a registered node but it's not connected:

### Scenario A: Node Shows OFFLINE
1. **Check power**: Is the ESP32-S3 powered on?
2. **Check WiFi**: Does the node have WiFi connectivity?
   - Look for the captive portal AP: `spaxel-XXXX` (last 4 of MAC)
   - If visible, connect to it and check/enter WiFi credentials
3. **Check mothership reachability**: Can the node ping the mothership?
   - Verify `ms_ip` NVS key (fallback if mDNS fails)
4. **Check logs**: Node logs available via serial (USB-Serial/JTAG)

### Scenario B: Node Shows STALE
1. **Node was connected but lost connection**
2. **Check WiFi stability**: Signal strength, interference
3. **Check mothership availability**: Has the mothership restarted recently?
4. **Node auto-reconnect**: Node should retry connection every 5 seconds

### Scenario C: No Nodes Registered
1. **Onboarding required**: Need to provision the first node
2. **Use Web Serial flow** from dashboard (requires USB connection to ESP32-S3)
3. **Verify provisioning payload** includes:
   - WiFi credentials (from Settings > Network)
   - Generated node token (HMAC-SHA256)
   - Node ID (UUID4)

### Passive Radar Mode Check

If the system is configured for passive radar mode (using home WiFi AP as signal source):
- Verify the firmware NVS key `passive_bss` holds the correct 6-byte home AP BSSID. (`passive_bssid` is the runtime role-message field, not the persisted NVS key.)
- Check that nodes are assigned `role: passive`
- Verify AP BSSID matches the actual router's BSSID
- Check CSI frames are being filtered to the AP's BSSID (not 00:00:00:00:00:00)

## Required Evidence for Closure

Closure evidence for this negative verification:
1. ✅ Mothership health confirmation (healthy, `nodes_online: 0`)
2. ✅ Absence of a connected first node established by the live health count
3. ✅ CSI streaming absence established: zero connected sessions means no frames; rate is 0/unavailable
4. ✅ Operator troubleshooting runbook written, including the `passive_bss` check

**Current State:** The node is not connected and is not streaming. Authenticated `/api/nodes` output is still needed only to distinguish “no registered node” from “registered but offline”; it is not needed to establish that the live mothership currently has no connected stream.

---

**Next Action:** Please provide authenticated curl output from `/api/nodes` so we can determine:
- Whether any nodes are registered in the system
- If a node exists, why it's not connected
- What troubleshooting steps to apply
