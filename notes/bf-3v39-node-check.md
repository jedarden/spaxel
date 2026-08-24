# Node Connection & CSI Streaming Verification

**Date:** 2026-08-24  
**Bead:** spaxel-9c1a4858 (split-child 1 of bf-3v39)  
**Mothership:** https://spaxel.ardenone.com  
**Purpose:** Verify first ESP32 node shows CONNECTED and CSI frames are arriving

## Current Status

### Mothership Health (from `/healthz` endpoint)
```json
{
  "status": "ok",
  "uptime_s": 791355,           // ~9.2 days uptime
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

### Finding Summary
**❌ NO NODES ARE CURRENTLY CONNECTED TO THE MOTHERSHIP.**

The mothership is healthy and has been running continuously for ~9.2 days, but zero nodes are online. This means:
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
- Verify `passive_bssid` NVS key holds the correct AP BSSID
- Check that nodes are assigned `role: passive`
- Verify AP BSSID matches the actual router's BSSID
- Check CSI frames are being filtered to the AP's BSSID (not 00:00:00:00:00:00)

## Required Evidence for Closure

To close this bead, we need:
1. ✅ Mothership health confirmation (DONE - shows healthy, no nodes)
2. ⏸️ Node status from `/api/nodes` (BLOCKED - requires OAuth)
3. ⏸️ CSI streaming confirmation (BLOCKED - requires node to be connected)
4. ⏸️ CSI frame rate/timestamps (BLOCKED - requires `/api/links` or similar)

**Current State:** Cannot complete verification without operator-provided authenticated API responses. The system is running but has zero connected nodes.

---

**Next Action:** Please provide authenticated curl output from `/api/nodes` so we can determine:
- Whether any nodes are registered in the system
- If a node exists, why it's not connected
- What troubleshooting steps to apply
