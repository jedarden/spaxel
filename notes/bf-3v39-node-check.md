# Node Connection and CSI Streaming Verification

**Date:** 2026-08-16
**Bead:** spaxel-9c1a4858 (split-child of bf-3v39)
**Mothership:** https://spaxel.ardenone.com

## Findings

### Live Mothership Status

Verified via `/healthz` endpoint (public, no authentication required):

```json
{
  "status": "ok",
  "uptime_s": 100057,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

**Key Finding: No ESP32 nodes are currently connected to the mothership.**

### API Access Investigation

The spaxel.ardenone.com API is OAuth-gated through Google SSO (auth.ardenone.com). Available endpoints:

- **Public (no auth):**
  - `/healthz` - System status (confirmed working)
  - `/api/provision` - Node provisioning/tokens
  - `/api/auth/status` - Auth configuration
  - `/api/auth/setup` - Initial PIN setup
  - `/api/auth/login` - PIN authentication

- **Authenticated (requires Google OAuth or PIN session):**
  - `/api/nodes` - List all nodes
  - `/api/fleet` - Extended node data with packet rates
  - `/api/nodes/{mac}` - Individual node details

The `/api/fleet` endpoint would show CSI streaming rates (`packet_rate` field per node), but it requires authentication.

### CSI Frame Tracking

From code analysis (`mothership/internal/recorder/manager.go`):
- CSI frames are tracked per-link via the recorder manager
- Frame rates would be visible through authenticated fleet endpoints
- System uses 1-hour segmented recording windows

## Root Cause

**No ESP32 nodes are provisioned or connected.** The mothership is running cleanly (database OK, ~27.8 hours uptime) but has zero connected nodes.

This is expected for a fresh deployment or lab setup - nodes must be:
1. Physically provisioned via Web Serial ( `/api/provision` )
2. Connected to the same WiFi network as the mothership
3. Powered on with valid NVS configuration

## Troubleshooting Runbook

To get the first ESP32 node connected and streaming CSI:

### Step 1: Verify Mothership Network Configuration

```bash
# Check if network WiFi credentials are configured (fleet-wide default per ADR-005)
# This would be visible through the Settings > Network page (requires OAuth)
# Nodes can be provisioned with custom WiFi credentials even if fleet-wide creds aren't set
```

The `/api/provision` endpoint generates node-specific provisioning payloads with:
- WiFi SSID/password (can be node-specific or fleet-wide)
- Node token (HMAC-SHA256 derived from install_secret + MAC)
- Mothership connection details (mDNS host/IP, port)

### Step 2: Provision First Node

Access https://spaxel.ardenone.com (requires Google OAuth via auth.ardenone.com), then:

1. Navigate to Settings → Nodes → "Add Node" (or use Fleet page if available)
2. Connect ESP32 via Web Serial
3. Provisioning payload is written to NVS including:
   - `wifi_ssid`, `wifi_pass`
   - `node_token` (for authentication)
   - `ms_mdns` (mothership mDNS hostname)
   - `ms_port` (default: 9000)
   - `ntp_server` (for time sync)
4. ESP32 reboots and connects to WiFi

### Step 3: Verify Node Connection

After provisioning, the node should:
1. Connect to WiFi
2. Resolve mothership via mDNS or direct IP
3. Send hello message with MAC + node_token
4. Ingestion server validates token
5. Node appears in `/api/fleet` with status "online"

Check via `/healthz`:
```json
{
  "nodes_online": 1,
  "reason": ""  // Empty when nodes are present
}
```

### Step 4: Verify CSI Streaming

Once connected, CSI streaming requires:

1. **Node role configuration** - Node must be in active role:
   - `tx` - Transmitter beaconing
   - `rx` - Receiver collecting CSI
   - `tx_rx` - Both transmitting and receiving
   - `passive` - Passive monitoring on specific BSSID **(requires passive_bssid field)**

2. **For passive mode** (for home AP monitoring):
   ```bash
   # Must set passive_bssid to home WiFi AP's BSSID
   curl -X POST https://spaxel.ardenone.com/api/nodes/{mac}/role \
     -H "Content-Type: application/json" \
     -d '{"role": "passive", "passive_bssid": "AA:BB:CC:DD:EE:FF"}'
   ```

   **Critical:** Without `passive_bssid`, passive mode filters CSI on `00:00:00:00:00:00` and drops all frames. See bf-6auk5 context.

3. **Verify CSI rate** via `/api/fleet`:
   ```json
   {
     "nodes": [{
       "mac": "...",
       "status": "online",
       "packet_rate": 19.8,  // CSI frames/sec
       "configured_rate": 20,
       "health_score": 0.95
     }]
   }
   ```

### Step 5: Common Issues

**Node not appearing in fleet:**
- Check ESP32 serial output for WiFi connection status
- Verify mothership mDNS resolvable from node's network
- Check node_token matches (derived from install_secret + MAC)
- Review `/healthz` for database errors

**Node shows "unpaired":**
- Node connected but token validation failed
- Re-provision node with correct MAC address
- Check install_secret consistency between provisioning and ingestion

**Node online but no CSI (packet_rate = 0):**
- Verify node role is not `idle` or `virtual`
- For passive mode, confirm `passive_bssid` is set to home AP BSSID
- Check WiFi channel - CSI requires node and AP on same channel
- Verify node is actually receiving frames from target AP

**CSI rate low (< 5 fps):**
- Normal for idle environments with minimal WiFi traffic
- Active TX beacons from another node increase CSI rate
- Check WiFi interference or channel congestion
- Verify node health_score > 0.8

## Next Steps

To complete bf-3v39 (presence detection verification), the operator needs to:

1. Access https://spaxel.ardenone.com via Google OAuth
2. Provision at least one ESP32 node with WiFi credentials
3. Configure node role for CSI collection (passive with home AP BSSID)
4. Verify node appears in fleet with non-zero packet_rate
5. Re-verify this bead once CSI streaming is confirmed

The mothership software is functioning correctly - this is purely a deployment/provisioning task.
