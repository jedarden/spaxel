# Bench Hostname Information

**Last Updated:** 2026-08-28
**Source:** `docs/deployment/bench-connectivity.md`

## Bench Machine Details

- **Full Hostname:** `bench.tail1b1987.ts.net`
- **Short Name:** `bench` (resolves via Tailscale MagicDNS)
- **Tailscale IP:** `100.67.12.56`
- **Service Port:** 6080 (designated for bench operations)
- **Network:** Tailscale VPN (`.ts.net` suffix)
- **Status:** ✅ Verified accessible (2026-08-28)

## Connectivity Validation

```bash
# DNS resolution
$ host bench
bench.tail1b1987.ts.net has address 100.67.12.56

# Port accessibility check
$ nc -zv bench.tail1b1987.ts.net 6080
Connection to bench.tail1b1987.ts.net (100.67.12.56) 6080 port [tcp/*] succeeded!
```

## Network Path

1. **Local machine** (`codinghome.ardenone.com`)
   - Resolves `bench` via Tailscale MagicDNS
   - Routes through Tailscale VPN (`100.67.12.56`)

2. **Bench machine** (`bench.tail1b1987.ts.net`)
   - Accepts connections on port 6080
   - All connectivity over encrypted Tailscale mesh network

## Usage Notes

- Bench machine is reachable via Tailscale VPN from any machine on the same tailnet
- Port 6080 is the designated service port for bench operations
- All connectivity is over the encrypted Tailscale mesh network
