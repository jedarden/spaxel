# Mothership IP Override (mDNS-less Networks)

## Problem

mDNS (multicast DNS, port 5353) is blocked or filtered on some networks:

- Enterprise WiFi with AP isolation
- Mesh WiFi systems that suppress multicast between bands
- VLAN-segmented networks where multicast traffic doesn't cross boundaries
- Networks with aggressive multicast-to-unicast conversion that breaks mDNS

On first boot, an ESP32 node has no cached mothership IP and relies entirely on mDNS discovery (`_spaxel._tcp`). If mDNS is blocked, the node enters `MOTHERSHIP_UNAVAILABLE` state and never connects.

## Solution: Provisioning-time IP override

The provisioning payload now supports an optional `ms_ip` field — a direct IPv4 address that bypasses mDNS on first connect.

### How it works

1. During provisioning (Web Serial wizard), the dashboard sends `ms_ip` in the POST body to `/api/provision`
2. The mothership includes it in the provisioning payload JSON
3. The firmware writes it to two NVS keys:
   - `ms_ip` — the runtime fallback cache (used on subsequent boots if mDNS fails)
   - `ms_ip_prov` — the provisioning-time override (used on first boot only)
4. On first boot, the discovery order is:
   1. **Provisioned IP** (`ms_ip_prov`) — tried first, skips mDNS entirely on first attempt
   2. **mDNS query** (`_spaxel._tcp`) — standard discovery
   3. **Cached IP** (`ms_ip`) — fallback from a previous successful connection

After a successful connection, `ms_ip` is updated to the current IP. The `ms_ip_prov` value persists but is only preferred on the very first connection attempt (`discovery_fail_count == 0`). If the provisioned IP fails, mDNS is tried next, then the cached IP.

### When to use the override

- The node is on a different VLAN/subnet from the mothership and mDNS doesn't cross boundaries
- Enterprise or campus WiFi that blocks multicast
- Mesh WiFi systems where nodes on different bands can't discover each other via mDNS
- Any network where `ping spaxel.local` from a computer fails but `ping <mothership-ip>` works

### When NOT to use it

- On normal home networks where mDNS works — leaving `ms_ip` blank lets the node adapt to DHCP IP changes automatically
- The mothership IP changes frequently — the cached `ms_ip` updates on each successful connection, but a stale provisioned IP would cause one failed attempt before falling back to mDNS

### Dashboard wizard

The Web Serial onboarding wizard has an "Advanced: Network Troubleshooting" section on the WiFi credentials step. The "Mothership IP" field is auto-populated when the browser is accessing the dashboard via IP address (e.g., `http://192.168.1.100:8080`). Users on mDNS-blocking networks should enter the mothership's LAN IP there.
